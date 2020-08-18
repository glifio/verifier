package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/filecoin-project/go-address"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func main() {
	fmt.Println("Lotus node: ", env.LotusAPIDialAddr)
	fmt.Println("Time before miner can reup: ", env.FaucetRateLimit)
	fmt.Println("Time before verifiers can reup: ", env.VerifierRateLimit)
	fmt.Println("First time miner faucet amount: ", env.FaucetFirstTimeMinerGrant)
	fmt.Println("Second time+ miner faucet amount: ", env.FaucetMinerGrant)
	fmt.Println("Non miner amount: ", env.FaucetNonMinerGrant)
	fmt.Println("Faucet min GH account age: ", env.FaucetMinAccountAge)
	fmt.Println("dynamodb table name: ", env.DynamodbTableName)

	// runTest()

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.POST("/oauth/:provider", serveOauth, handleError("/oauth"))
	router.POST("/verify", serveVerifyAccount)
	router.GET("/verifiers", serveListVerifiers)
	router.GET("/verified-clients", serveListVerifiedClients)
	router.GET("/account-remaining-bytes/:target_addr", serveCheckAccountRemainingBytes)
	router.GET("/verifier-remaining-bytes/:target_addr", serveCheckVerifierRemainingBytes)
	router.POST("/faucet/:target_addr", serveFaucet, handleError("/faucet"))

	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider  = errors.New("unsupported oauth provider")
	ErrUserTooNew           = errors.New("user account is too new")
	ErrSufficientAllowance  = errors.New("allowance is already sufficient")
	ErrAllocatedTooRecently = errors.New("you must wait 30 days in between reallocations")
)

type UserLock string

var (
	UserLock_Verifier UserLock = "Verifier"
	UserLock_Faucet   UserLock = "Faucet"
)

func setError(c *gin.Context, code int, err error) {
	c.Set("error", err)
	c.Set("code", code)
}

func handleError(route string) gin.HandlerFunc {
	return func(c *gin.Context) {
		err, hasErr := c.Get("error")
		code, hasCode := c.Get("code")
		if hasErr {
			if hasCode {
				c.JSON(code.(int), gin.H{"error": err.(error).Error()})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.(error).Error()})
			}
			log.Printf("%v error: %+v", route, err)
		}
	}
}

func serveOauth(c *gin.Context) {
	providerName := c.Param("provider")
	provider, exists := oauthProviders[providerName]
	if !exists {
		setError(c, http.StatusBadRequest, errors.Wrapf(ErrUnsupportedProvider, "provider=%v", providerName))
		return
	}

	type Request struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		setError(c, http.StatusBadRequest, errors.Wrap(err, "binding request JSON"))
		return
	}

	// Exchange the `code` for an `access_token`
	token, err := OAuthExchangeCodeForToken(provider, body.Code, body.State)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "exchanging code for token"))
		return
	}

	// Fetch the user's profile
	accountData, err := provider.FetchAccountData(token)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "fetching account data"))
		return
	}

	// Update user record in Dynamo
	user, err := getUserWithProviderUniqueID(providerName, accountData.UniqueID)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "fetching DynamoDB user"))
		return
	}

	user.Accounts[providerName] = accountData

	err = saveUser(user)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "saving DynamoDB user"))
		return
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID": user.ID,
		"nbf":    time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	jwtTokenString, err := jwtToken.SignedString([]byte(env.JWTSecret))
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "generating JWT"))
		return
	}

	type Response struct {
		JWT string `json:"jwt"`
	}

	c.JSON(http.StatusOK, Response{jwtTokenString})
}

func serveVerifyAccount(c *gin.Context) {
	type Request struct {
		TargetAddr string `json:"targetAddr" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := getUserIDFromJWT(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// This helps us keep the user locked while we wait to see if the message was successful.  If
	// we don't reach the point where we've submitted it, we go ahead and unlock the user right away.
	var successfullySubmittedMessage bool

	// Lock the user for the duration of this operation
	err = lockUser(userID, UserLock_Verifier)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		if !successfullySubmittedMessage {
			unlockUser(userID, UserLock_Verifier)
		}
	}()

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user not found, have you authenticated?"})
		return
	}

	// Ensure that the user's account is old enough
	minAccountAge := time.Duration(env.VerifierMinAccountAgeDays) * 24 * time.Hour
	if !user.HasAccountOlderThan(minAccountAge) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Ensure that the user hasn't asked for more allocation too recently
	if user.MostRecentAllocation.Add(env.VerifierRateLimit * time.Hour).After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAllocatedTooRecently.Error()})
		return
	}

	// Ensure that the user is actually owed bytes
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	remaining, err := lotusCheckAccountRemainingBytes(ctx, body.TargetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	owed := big.Sub(env.MaxAllowanceBytes, remaining)
	if big.Cmp(owed, big.NewInt(0)) <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you have verified data already, Greedy McRichbags"})
		return
	}

	// Allocate the bytes
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cid, err := lotusVerifyAccount(ctx, body.TargetAddr, owed.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	successfullySubmittedMessage = true

	// Respond to the HTTP request
	type Response struct {
		Cid string `json:"cid"`
	}
	c.JSON(http.StatusOK, Response{Cid: cid.String()})

	go func() {
		defer unlockUser(userID, UserLock_Verifier)

		// Determine whether the Filecoin message succeeded
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		ok, err := lotusWaitMessageResult(ctx, cid)
		if err != nil {
			// This is already logged in lotusWaitMessageResult
			return
		}
		user.VerifiedFilecoinAddress = body.TargetAddr
		if ok {
			user.MostRecentAllocation = time.Now()
		}
		err = saveUser(user)
		if err != nil {
			log.Println("error saving user:", err)
		}
	}()
}

func serveListVerifiers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	verifiers, err := lotusListVerifiers(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, verifiers)
}

func serveListVerifiedClients(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	verifiedClients, err := lotusListVerifiedClients(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, verifiedClients)
}

func serveCheckAccountRemainingBytes(c *gin.Context) {
	targetAddr := c.Param("target_addr")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dcap, err := lotusCheckAccountRemainingBytes(ctx, targetAddr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if dcap.Int == nil {
		dcap = big.NewInt(0)
	}

	user, err := getUserByVerifiedFilecoinAddress(targetAddr)
	if err != nil {
		// no-op
	}

	type Response struct {
		RemainingBytes       string    `json:"remainingBytes"`
		MostRecentAllocation time.Time `json:"mostRecentAllocation"`
	}
	c.JSON(http.StatusOK, Response{dcap.String(), user.MostRecentAllocation})
}

func serveCheckVerifierRemainingBytes(c *gin.Context) {
	targetAddr := c.Param("target_addr")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dcap, err := lotusCheckVerifierRemainingBytes(ctx, targetAddr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dcap)
}

func serveFaucet(c *gin.Context) {
	targetAddrStr := c.Param("target_addr")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	targetAddr, err := address.NewFromString(targetAddrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "getting full node API"))
		return
	}
	defer closer()

	faucetAddr := env.FaucetAddr
	if faucetAddr == (address.Address{}) {
		faucetAddr, err = api.WalletDefaultAddress(ctx)
		if err != nil {
			setError(c, http.StatusInternalServerError, errors.Wrap(err, "getting wallet default address"))
			return
		}
	}

	userID, err := getUserIDFromJWT(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// This helps us keep the user locked while we wait to see if the message was successful.  If
	// we don't reach the point where we've submitted it, we go ahead and unlock the user right away.
	var successfullySubmittedMessage bool

	// Lock the user for the duration of this operation
	err = lockUser(userID, UserLock_Faucet)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "your last transaction is still confirming"})
		return
	}
	defer func() {
		if !successfullySubmittedMessage {
			unlockUser(userID, UserLock_Faucet)
		}
	}()

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user not found, have you authenticated?"})
		return
	}

	// No account less than MinAccountAge is allowed any FIL
	if !user.HasAccountOlderThan(env.FaucetMinAccountAge) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// returns an ID address if its a miner address, otherwise an empty address
	minerAddr, err := lotusGetMinerAddr(ctx, targetAddr)
	if err != nil && errors.Cause(err) != ErrNotMiner {
		setError(c, http.StatusInternalServerError, errors.Wrapf(err, "getting miner address for %v", targetAddr))
		return
	}

	isMiner := !minerAddr.Empty()

	// ensure the non-miner/new miner hasn't already gotten their non-miner faucet tx
	if !isMiner && user.ReceivedNonMinerFaucetGrant {
		c.JSON(http.StatusForbidden, gin.H{"error": "non-miners can only use the faucet once"})
		return
	}

	// assume this account is not a miner
	owed := env.FaucetNonMinerGrant

	if isMiner {
		if user.HasRequestedFromFaucetAsMiner() {
			if user.MostRecentMinerFaucetGrant.Add(env.FaucetRateLimit).After(time.Now()) {
				c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("miners may only use the faucet once every %v hours", env.FaucetRateLimit.Hours())})
				return
			}
			owed = env.FaucetMinerGrant
		} else {
			owed = env.FaucetFirstTimeMinerGrant
		}

		worker, err := lotusGetMinerWorker(ctx, minerAddr)
		if err != nil {
			setError(c, http.StatusInternalServerError, errors.Wrapf(err, "getting miner worker for %v", minerAddr))
			return
		}

		targetAddr = worker
	}

	cid, err := lotusSendFIL(ctx, faucetAddr, targetAddr, owed)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrapf(err, "sending %v from %v to %v", owed, faucetAddr, targetAddr))
		return
	}

	successfullySubmittedMessage = true

	// Respond to the HTTP request
	type Response struct {
		Cid     string `json:"cid"`
		Sent    string `json:"sent"`
		Address string `json:"toAddress"`
	}
	c.JSON(http.StatusOK, Response{
		Cid:     cid.String(),
		Sent:    owed.String(),
		Address: targetAddr.String(),
	})

	go func() {
		defer unlockUser(userID, UserLock_Faucet)

		// Determine whether the Filecoin message succeeded
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		ok, err := lotusWaitMessageResult(ctx, cid)
		if err != nil {
			// This is already logged in lotusWaitMessageResult
			return
		} else if !ok {
			// Transaction failed
			log.Println("ERROR: faucet transaction failed")
			return
		}

		user.MostRecentFaucetGrantCid = cid.String()
		user.MostRecentFaucetAddress = targetAddr.String()
		if !minerAddr.Empty() {
			user.MostRecentMinerFaucetGrant = time.Now()
		} else {
			user.ReceivedNonMinerFaucetGrant = true
		}

		err = saveUser(user)
		if err != nil {
			log.Println("error saving user:", err)
		}
	}()
}

func getUserIDFromJWT(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("bad Authorization header")
	}

	jwtToken := strings.TrimSpace(authHeader[len("Bearer "):])

	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(env.JWTSecret), nil
	})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", err
	}

	userID, ok := claims["userID"].(string)
	if !ok {
		return "", err
	}
	return userID, nil
}
