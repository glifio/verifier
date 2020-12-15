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
	"github.com/filecoin-project/go-state-types/big"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func registerVerifierHandlers(router *gin.Engine) {
	router.POST("/verify/:target_addr", serveVerifyAccount)
	router.GET("/verifiers", serveListVerifiers)
	router.GET("/verified-clients", serveListVerifiedClients)
	router.GET("/account-remaining-bytes/:target_addr", serveCheckAccountRemainingBytes)
	router.GET("/verifier-remaining-bytes/:target_addr", serveCheckVerifierRemainingBytes)
}

func main() {
	fmt.Println("Lotus node: ", env.LotusAPIDialAddr)
	fmt.Println("Time before miner can reup: ", env.FaucetRateLimit)
	fmt.Println("Time before verifiers can reup: ", env.VerifierRateLimit)
	fmt.Println("First time miner faucet amount: ", env.FaucetFirstTimeMinerGrant)
	fmt.Println("Second time+ miner faucet amount: ", env.FaucetMinerGrant)
	fmt.Println("Non miner amount: ", env.FaucetNonMinerGrant)
	fmt.Println("Faucet min GH account age: ", env.FaucetMinAccountAge)
	fmt.Println("dynamodb table name: ", env.DynamodbTableName)
	fmt.Println("Max fee: ", env.MaxFee)

	err := initBlockListCache()
	if err != nil {
		fmt.Println("ERROR CREATING BLOCKLIST: ", err)
	}
	
	router := gin.Default()
	if _, err = instantiateWallet(&gin.Context{}); err != nil {
		log.Panic("ERROR INSTANTIATING WALLET: ", err)
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.POST("/oauth/:provider", serveOauth, handleError("/oauth"))

	if env.Mode == FaucetMode {
		router.POST("/faucet/:target_addr", serveFaucet, handleError("/faucet"))
	} else if env.Mode == VerifierMode {
		registerVerifierHandlers(router)
	} else {
		router.POST("/faucet/:target_addr", serveFaucet, handleError("/faucet"))
		registerVerifierHandlers(router)
	}

	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider  = errors.New("unsupported oauth provider")
	ErrUserTooNew           = errors.New("User account is too new.")
	ErrSufficientAllowance  = errors.New("allowance is already sufficient")
	ErrAllocatedTooRecently = errors.New("you must wait 30 days in between reallocations")
	ErrStaleJWT             = errors.New("The network has reset since your last visit. Please click the retry button above.")
	ErrFaucetRepeatAttempt  = errors.New("This GitHub account has already used the faucet.")
	ErrUserLocked           = errors.New("We're still waiting for your previous transaction to finalize.")
	ErrAddressBlocked       = errors.New("This address or Miner ID has reached its maximum usage of the faucet.")
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

func serveWallet(c *gin.Context) {
	type Response struct {
	}

	c.JSON(http.StatusOK, Response{})
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
	targetAddrStr := c.Param("target_addr")
	
	userID, err := getUserIDFromJWT(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
		return
	}

	if len(user.Accounts) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
		return
	}

	if user.Locked_Verifier {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserLocked.Error()})
		return
	}

	// Ensure that the user's account is old enough
	minAccountAge := time.Duration(env.VerifierMinAccountAgeDays) * 24 * time.Hour
	if !user.HasAccountOlderThan(minAccountAge) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Ensure that the user hasn't asked for more allocation too recently
	if user.MostRecentAllocation.Add(env.VerifierRateLimit).After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAllocatedTooRecently.Error()})
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

	// Ensure that the user is actually owed bytes
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	remaining, err := lotusCheckAccountRemainingBytes(ctx, targetAddrStr)
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

	cid, err := lotusVerifyAccount(ctx, targetAddrStr, owed.String())
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
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()

		ok, err := lotusWaitMessageResult(ctx, cid)
		if err != nil {
			// This is already logged in lotusWaitMessageResult
			return
		} else if !ok {
			// Transaction failed
			log.Println("ERROR: verify transaction failed")
			return
		}

		user.VerifiedFilecoinAddress = targetAddrStr
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
	userID, err := getUserIDFromJWT(c)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
		return
	}

	if len(user.Accounts) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
		return
	}

	if user.Locked_Faucet {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserLocked.Error()})
		return
	}

	if user.ReceivedNonMinerFaucetGrant {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrFaucetRepeatAttempt.Error()})
		return
	}

	// This helps us keep the user locked while we wait to see if the message was successful.  If
	// we don't reach the point where we've submitted it, we go ahead and unlock the user right away.
	var successfullySubmittedMessage bool

	// Lock the user for the duration of this operation
	err = lockUser(userID, UserLock_Faucet)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserLocked.Error()})
		return
	}
	defer func() {
		if !successfullySubmittedMessage {
			unlockUser(userID, UserLock_Faucet)
		}
	}()

	targetAddrStr := c.Param("target_addr")

	// No account less than MinAccountAge is allowed any FIL
	if !user.HasAccountOlderThan(env.FaucetMinAccountAge) {
		slackNotification := "Requester's FIL address: " + targetAddrStr + "\nRequester's GH Handle: " + user.Accounts["github"].Username + "\nRequester's Account age: " + user.Accounts["github"].CreatedAt.String() + "\n----------"
		sendSlackNotification("https://errors.glif.io/verifier-account-too-young", slackNotification)
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	targetAddr, err := address.NewFromString(targetAddrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if isAddressBlocked(targetAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAddressBlocked.Error()})
		return
	}

	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrap(err, "getting full node API"))
		return
	}
	defer closer()

	cid, err := lotusSendFIL(context.TODO(), api, FaucetAddr, targetAddr, env.FaucetNonMinerGrant)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrapf(err, "sending %v from %v to %v", env.FaucetNonMinerGrant, FaucetAddr, targetAddr))
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
		Sent:    env.FaucetNonMinerGrant.String(),
		Address: targetAddr.String(),
	})

	go func() {
		defer unlockUser(userID, UserLock_Faucet)

		// Determine whether the Filecoin message succeeded
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Minute)
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
		user.MostRecentFaucetAddress = targetAddrStr
		// we're using this db variable for now to track all users to stay backwards compat w space race
		user.ReceivedNonMinerFaucetGrant = true

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
