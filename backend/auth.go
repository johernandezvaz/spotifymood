package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

type SpotifyTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type SpotifyUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Images      []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type Claims struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	jwt.RegisteredClaims
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func handleSpotifyLogin(c *fiber.Ctx) error {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")

	state := generateState()

	// Store state in session/cache for verification
	scope := "playlist-read-private playlist-read-collaborative playlist-modify-public playlist-modify-private user-read-private user-read-email"

	spotifyURL := fmt.Sprintf("https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s",
		clientID, url.QueryEscape(redirectURI), url.QueryEscape(scope), state)

	return c.JSON(fiber.Map{
		"auth_url": spotifyURL,
		"state":    state,
	})
}

func handleSpotifyCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Authorization code not provided",
		})
	}

	// Exchange code for tokens
	tokenResp, err := exchangeCodeForTokens(code)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to exchange code for tokens",
		})
	}

	// Get user profile
	user, err := getSpotifyUserProfile(tokenResp.AccessToken)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get user profile",
		})
	}

	// Save user to database
	dbUser := User{
		SpotifyID:    user.ID,
		DisplayName:  user.DisplayName,
		Email:        user.Email,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenExpiry:  time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if len(user.Images) > 0 {
		dbUser.ProfileImage = user.Images[0].URL
	}

	saveUser(&dbUser)

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID:      user.ID,
		AccessToken: tokenResp.AccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	})

	jwtSecret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create token",
		})
	}

	// Redirect to frontend with token
	frontendURL := os.Getenv("FRONTEND_URL")
	return c.Redirect(fmt.Sprintf("%s/auth/success?token=%s", frontendURL, tokenString))
}

func exchangeCodeForTokens(code string) (*SpotifyTokenResponse, error) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	client := &http.Client{}
	req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp SpotifyTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func getSpotifyUserProfile(accessToken string) (*SpotifyUser, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", "https://api.spotify.com/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user SpotifyUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func authMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(401).JSON(fiber.Map{
			"error": "Authorization header required",
		})
	}

	tokenString := strings.Replace(authHeader, "Bearer ", "", 1)

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return c.Status(401).JSON(fiber.Map{
			"error": "Invalid token",
		})
	}

	c.Locals("user_id", claims.UserID)
	c.Locals("access_token", claims.AccessToken)
	return c.Next()
}

func handleLogout(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

func getCurrentUser(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	user := getUserBySpotifyID(userID)

	if user == nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(fiber.Map{
		"user": fiber.Map{
			"id":            user.SpotifyID,
			"display_name":  user.DisplayName,
			"email":         user.Email,
			"profile_image": user.ProfileImage,
		},
	})
}
