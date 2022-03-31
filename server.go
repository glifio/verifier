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
	"github.com/glifio/go-logger"
	"github.com/pkg/errors"
	"gopkg.in/robfig/cron.v2"
)

func startFaucet(router *gin.Engine, c *cron.Cron) {
	logger.Infof("Faucet address: %v", FaucetAddr.String())
	logger.Infof("Faucet grant size: %v", env.FaucetGrantSize)
	logger.Infof("Faucet min GH account age days: %v", env.FaucetMinAccountAgeDays)

	// Add routes
	router.POST("/faucet/:target_addr", serveFaucet, handleError("/faucet"))

	// Add cron jobs
	c.AddFunc("@hourly", reconcileFaucetMessages)
}

func startVerifier(router *gin.Engine, c *cron.Cron) {
	logger.Infof("Verifier address: %v", VerifierAddr.String())
	logger.Infof("Verifier min GH account age days: %v", env.VerifierMinAccountAgeDays)
	logger.Infof("Verifier rate limit: %v", env.VerifierRateLimit)
	logger.Infof("Verifier base allowance: %v", env.BaseAllowanceBytes)
	logger.Infof("Max allocations: %v", env.MaxTotalAllocations)

	// Init counter
	err := initCounter(&gin.Context{})
	if err != nil {
		logger.Errorf("REDIS INIT COUNT FAILED: %v", err)
	}

	// Add routes
	router.POST("/verify/:target_addr", serveVerifyAccount)
	router.PUT("/verify/counter/:pwd", serveResetCounter)
	router.GET("/verify/counter/:pwd", serveCurrentCount)
	router.GET("/verifiers", serveListVerifiers)
	router.GET("/verified-clients", serveListVerifiedClients)
	router.GET("/allowance/:target_addr", serveAllowance)
	router.GET("/allowance-github/:github_user/:target_addr", serveAllowanceGithub)
	router.GET("/account-remaining-bytes/:target_addr", serveCheckAccountRemainingBytes)
	router.GET("/verifier-remaining-bytes/:target_addr", serveCheckVerifierRemainingBytes)

	// Add cron jobs
	c.AddFunc("@hourly", reconcileVerifierMessages)
}

func main() {
	// Initialize logger
	err := logger.Init(logger.LoggerOptions{
		ModuleName:    "verifier",
		SentryEnabled: env.SentryDsn != "",
		SentryDsn:     env.SentryDsn,
		SentryEnv:     env.SentryEnv,
		SentryLevel:   logger.LogLevelWarning,
		SentryTraces:  0,
	})
	if err != nil {
		log.Panic(err)
	}

	logger.Infof("Lotus node: %v", env.LotusAPIDialAddr)
	logger.Infof("Dynamodb table name: %v", env.DynamodbTableName)
	logger.Infof("Max transaction fee: %v", env.MaxFee)
	logger.Infof("Mode: %v", env.Mode)

	if err := initBlockListCache(); err != nil {
		log.Panic(err)
	}
	if _, err := instantiateWallet(&gin.Context{}); err != nil {
		log.Panic(err)
	}

	// Create Gin engine
	router := gin.Default()
	if logger.IsSentryEnabled() {
		router.Use(logger.GetSentryGin())
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"POST"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Add generic routes
	router.GET("/", servePong)
	router.GET("/healthz", servePong)
	router.GET("/ping", servePong)
	router.POST("/oauth/:provider", serveOauth, handleError("/oauth"))

	// Add app-specific routes
	c := cron.New()
	if env.Mode == FaucetMode {
		startFaucet(router, c)
	} else if env.Mode == VerifierMode {
		startVerifier(router, c)
	} else {
		startFaucet(router, c)
		startVerifier(router, c)
	}

	// Start cron jobs
	c.Start()
	defer func() {
		c.Stop()
	}()

	// Start Gin server
	router.Run(":" + env.Port)
}

var (
	ErrUnsupportedProvider  = errors.New("unsupported oauth provider")
	ErrUserTooNew           = errors.New("User account is too new.")
	ErrVerifiedClientExists = errors.New("This Filecoin address is already a verified client. Please try again with a new Filecoin address.")
	ErrAllocatedTooRecently = errors.New("You must wait 30 days in between reallocations")
	ErrStaleJWT             = errors.New("The network has reset since your last visit. Please click the retry button above.")
	ErrFaucetRepeatAttempt  = errors.New("This GitHub account has already used the faucet.")
	ErrUserLocked           = errors.New("Our servers are processing your last transaction. Come back tomorrow.")
	ErrAddressBlocked       = errors.New("This address or Miner ID has reached its maximum usage of the faucet.")
	ErrCounterReached       = errors.New("This notary has run out of data cap for today! Come back tomorrow.")
	ErrMaxAllowanceFailed   = errors.New("Failed to calculate the maximum allowance for the user account and filecoin address")
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
			logger.Errorf("%v: %v", route, err)
		}
	}
}

func servePong(c *gin.Context) {
	c.JSON(http.StatusOK, "pong")
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

	targetAddrStr := c.Param("target_addr")

	// Ensure that the user hasn't used this address before
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Ensure that the user's account is old enough
	minAccountAge := time.Duration(env.VerifierMinAccountAgeDays) * 24 * time.Hour
	// No account less than MinAccountAge is allowed any FIL
	if !user.HasAccountOlderThan(minAccountAge) {
		accountName := user.Accounts["github"].Username
		accountAge := user.Accounts["github"].CreatedAt.String()
		logger.Errorf("ACCOUNT TOO NEW: User ID %q, FIL Address %q, Account name %q, Account age %q", user.ID, targetAddrStr, accountName, accountAge)
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Ensure that the user hasn't asked for more allocation too recently
	if user.MostRecentAllocation.Add(env.VerifierRateLimit).After(time.Now()) {
		accountName := user.Accounts["github"].Username
		lastAllocation := user.MostRecentAllocation.String()
		logger.Errorf("REALLOCATION TOO SOON: User ID %q, FIL Address %q, Account name %q, Last allocation %q", user.ID, targetAddrStr, accountName, lastAllocation)
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAllocatedTooRecently.Error()})
		return
	}

	// Lock the user for the duration of this operation until cron job cleans it up
	err = lockUser(userID, UserLock_Verifier)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserLocked.Error()})
		return
	}

	user, err = getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
		return
	}

	reachedCount, err := reachedCounter(c)
	if reachedCount {
		logger.Errorf("VERIFIER COUNTER REACHED: %v", env.MaxTotalAllocations)
		c.JSON(http.StatusLocked, gin.H{"error": ErrCounterReached.Error()})
		return
	}

	if err != nil {
		logger.Errorf("VERIFIER COUNTER CALCULATION FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": ErrCounterReached.Error()})
		return
	}

	dataCap, err := lotusCheckVerifierRemainingBytes(c, VerifierAddr.String())
	if err != nil {
		logger.Errorf("LOTUS CHECK VERIFIER BYTES FAILED: %v", err)
		c.JSON(http.StatusLocked, gin.H{"error": ErrCounterReached.Error()})
		return
	}

	fiftyDataCaps := big.Mul(getMaxAllowance(), big.NewInt(50))
	if dataCap.LessThanEqual(fiftyDataCaps) {
		logger.Warningf("LOW DATA CAP: %v", dataCap.String())
	}

	targetAddr, err := address.NewFromString(targetAddrStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if isAddressBlocked(targetAddr) {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrAddressBlocked.Error()})
		return
	}

	// Get maximum allowance for user / address combination
	allowance, err := user.GetAllowance(targetAddrStr)
	if err != nil {
		logger.Errorf("CALCULATING MAX ALLOWANCE FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": ErrMaxAllowanceFailed.Error()})
		return
	}

	// Allocate the bytes
	err = incrementCounter(c)
	if err != nil {
		logger.Errorf("REDIS INCREMENT COUNT FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel = context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	cid, err := lotusVerifyAccount(ctx, targetAddrStr, allowance)
	if err != nil {
		logger.Errorf("LOTUS VERIFY ACCOUNT FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user.MostRecentDataCapCid = cid.String()
	user.MostRecentVerifiedAddress = targetAddrStr

	err = saveUser(user)
	if err != nil {
		// TODO what to do here?
		logger.Errorf("ERROR SAVING USER: %v", err)
	}

	// Respond to the HTTP request
	type Response struct {
		Cid       string `json:"cid"`
		Allowance string `json:"allowance"`
	}
	c.JSON(http.StatusOK, Response{
		Cid:       cid.String(),
		Allowance: allowance.String(),
	})
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

func serveAllowance(c *gin.Context) {
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

	// Get the allowance for the user
	targetAddr := c.Param("target_addr")
	allowance, err := user.GetAllowance(targetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Respond with allowance
	type Response struct {
		Allowance string `json:"allowance"`
	}
	c.JSON(http.StatusOK, Response{Allowance: allowance.String()})
}

func serveAllowanceGithub(c *gin.Context) {
	// Get the allowance for the user
	targetAddr := c.Param("target_addr")
	githubUser := c.Param("github_user")
	allowance, err := getAllowanceGithub(githubUser, targetAddr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Respond with allowance
	type Response struct {
		Allowance string `json:"allowance"`
	}
	c.JSON(http.StatusOK, Response{Allowance: allowance.String()})
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

	type Response struct {
		RemainingBytes string `json:"remainingBytes"`
	}
	c.JSON(http.StatusOK, Response{dcap.String()})
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

	// This can get deleted, along with the `ReceivedFaucetGrant` key in dynamo if the faucet policy changes away from 1 time use only
	if user.ReceivedFaucetGrant {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrFaucetRepeatAttempt.Error()})
		return
	}

	targetAddrStr := c.Param("target_addr")

	minAccountAge := time.Duration(env.FaucetMinAccountAgeDays) * 24 * time.Hour
	// No account less than MinAccountAge is allowed any FIL
	if !user.HasAccountOlderThan(minAccountAge) {
		accountName := user.Accounts["github"].Username
		accountAge := user.Accounts["github"].CreatedAt.String()
		logger.Errorf("ACCOUNT TOO NEW: User ID %q, FIL Address %q, Account name %q, Account age %q", user.ID, targetAddrStr, accountName, accountAge)
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserTooNew.Error()})
		return
	}

	// Lock the user for the duration of this operation
	err = lockUser(userID, UserLock_Faucet)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrUserLocked.Error()})
		return
	}

	user, err = getUserByID(userID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": ErrStaleJWT.Error()})
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

	cid, err := lotusSendFIL(context.TODO(), api, FaucetAddr, targetAddr, env.FaucetGrantSize)
	if err != nil {
		setError(c, http.StatusInternalServerError, errors.Wrapf(err, "sending %v from %v to %v", env.FaucetGrantSize, FaucetAddr, targetAddr))
		return
	}

	user.MostRecentFaucetGrantCid = cid.String()
	user.MostRecentFaucetAddress = targetAddrStr

	err = saveUser(user)
	if err != nil {
		logger.Errorf("ERR FOR NEW RELIC: %v", err)
	}

	// Respond to the HTTP request
	type Response struct {
		Cid     string `json:"cid"`
		Sent    string `json:"sent"`
		Address string `json:"toAddress"`
	}
	c.JSON(http.StatusOK, Response{
		Cid:     cid.String(),
		Sent:    env.FaucetGrantSize.String(),
		Address: targetAddr.String(),
	})
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

func serveResetCounter(c *gin.Context) {
	password := c.Param("pwd")
	if password != env.AllocationsCounterResetPword {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not allowed"})
		return
	}
	if _, err := resetCounter(c); err != nil {
		logger.Errorf("REDIS RESET COUNT FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, "")
}

func serveCurrentCount(c *gin.Context) {
	password := c.Param("pwd")
	if password != env.AllocationsCounterResetPword {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not allowed"})
		return
	}
	count, err := getCount(c)
	if err != nil {
		logger.Errorf("REDIS GET COUNT FAILED: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, count)
}
