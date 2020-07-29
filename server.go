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
	"github.com/filecoin-project/lotus/chain/types"
	big "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func main() {
	fmt.Println("Lotus node:", env.LotusAPIDialAddr)

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.POST("/oauth/:provider", serveOauth)
	router.POST("/make-verifier", serveMakeVerifier)
	router.POST("/verify", serveVerifyAccount)
	router.GET("/verifiers", serveListVerifiers)
	router.GET("/verified-clients", serveListVerifiedClients)
	router.GET("/account-remaining-bytes/:target_addr", serveCheckAccountRemainingBytes)
	router.GET("/verifier-remaining-bytes/:target_addr", serveCheckVerifierRemainingBytes)
	router.GET("/balance/:target_addr", serveGetBalance)
	router.POST("/faucet/:target_addr", serveFaucet)

	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider  = errors.New("unsupported oauth provider")
	ErrUserTooNew           = errors.New("user account is too new")
	ErrSufficientAllowance  = errors.New("allowance is already sufficient")
	ErrAllocatedTooRecently = errors.New("you must wait 30 days in between reallocations")
)

func serveOauth(c *gin.Context) {
	providerName := c.Param("provider")
	provider, exists := oauthProviders[providerName]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrUnsupportedProvider.Error()})
		return
	}

	type Request struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Exchange the `code` for an `access_token`
	token, err := OAuthExchangeCodeForToken(provider, body.Code, body.State)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch the user's profile
	accountData, err := provider.FetchAccountData(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update user record in Dynamo
	user, err := getUserWithProviderUniqueID(providerName, accountData.UniqueID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetching DynamoDB user: " + err.Error()})
		return
	}

	user.Accounts[providerName] = accountData

	err = saveUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saving DynamoDB user: " + err.Error()})
		return
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID": user.ID,
		"nbf":    time.Date(2015, 10, 10, 12, 0, 0, 0, time.UTC).Unix(),
	})

	// Sign and get the complete encoded token as a string using the secret
	jwtTokenString, err := jwtToken.SignedString([]byte(env.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generating JWT: " + err.Error()})
		return
	}

	type Response struct {
		JWT string `json:"jwt"`
	}

	c.JSON(http.StatusOK, Response{jwtTokenString})
}

func serveMakeVerifier(c *gin.Context) {
	type Request struct {
		TargetAddr string `json:"targetAddr" binding:"required"`
		Allowance  string `json:"allowance" binding:"required"`
	}

	var body Request
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := lotusMakeAccountAVerifier(ctx, body.TargetAddr, body.Allowance)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
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
	err = lockUser(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		if !successfullySubmittedMessage {
			unlockUser(userID)
		}
	}()

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user not found, have you authenticated?"})
		return
	}

	// Ensure that the user's account is old enough
	var foundOne bool
	for _, account := range user.Accounts {
		if time.Now().Sub(account.CreatedAt).Hours() >= (time.Duration(env.MinAccountAgeDays) * 24 * time.Hour).Hours() {
			foundOne = true
			break
		}
	}
	if !foundOne {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Ensure that the user hasn't asked for more allocation too recently
	if user.MostRecentAllocation.Add(30 * 24 * time.Hour).After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAllocatedTooRecently.Error()})
		return
	}

	// Ensure that the user is actually owed bytes
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
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
		defer unlockUser(userID)

		// Determine whether the Filecoin message succeeded
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		ok, err := lotusWaitMessageResult(ctx, cid)
		if err != nil {
			// This is already logged in lotusWaitMessageResult
			return
		}
		user.FilecoinAddress = body.TargetAddr
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

	user, err := getUserByFilecoinAddress(targetAddr)
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

func serveGetBalance(c *gin.Context) {
	targetAddrStr := c.Param("target_addr")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetAddr, err := address.NewFromString(targetAddrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	balance, err := lotusCheckBalance(ctx, targetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user, err := getUserByFilecoinAddress(targetAddr.String())
	if err != nil {
		// no-op
	}

	type Response struct {
		Balance               string    `json:"balance"`
		MostRecentFaucetGrant time.Time `json:"mostRecentFaucetGrant"`
	}

	c.JSON(http.StatusOK, Response{balance.String(), user.MostRecentFaucetGrant})
}

func serveFaucet(c *gin.Context) {
	targetAddrStr := c.Param("target_addr")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targetAddr, err := address.NewFromString(targetAddrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	api, closer, err := lotusGetFullNodeAPI(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer closer()

	faucetAddr := env.FaucetAddr
	if faucetAddr == (address.Address{}) {
		faucetAddr, err = api.WalletDefaultAddress(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	err = lockUser(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		if !successfullySubmittedMessage {
			unlockUser(userID)
		}
	}()

	user, err := getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "user not found, have you authenticated?"})
		return
	}

	// Ensure the user isn't spamming the faucet
	if user.MostRecentFaucetGrant.Add(env.FaucetRatelimitInHours).After(time.Now()) {
		c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("you may only use the faucet once every %v", env.FaucetRatelimitInHours)})
		return
	}

	balance, err := lotusCheckBalance(ctx, targetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	owed := types.BigSub(types.BigInt(env.MaxAllowanceFIL), types.BigInt(balance))
	if types.BigCmp(types.NewInt(0), types.NewInt(0)) == -1 {
		c.JSON(http.StatusForbidden, gin.H{"error": "user is greedy"})
		return
	}

	cid, err := lotusSendFIL(ctx, faucetAddr, targetAddr, env.FaucetGasPrice, types.FIL(owed))
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
		defer unlockUser(userID)

		// Determine whether the Filecoin message succeeded
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancel()

		ok, err := lotusWaitMessageResult(ctx, cid)
		if err != nil {
			// This is already logged in lotusWaitMessageResult
			return
		}
		if ok {
			user.MostRecentFaucetGrant = time.Now()
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
