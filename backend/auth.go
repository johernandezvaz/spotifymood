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
	UserId     string `json:"user_id"`
	AccesToken string `json:"access_token"`
	jwt.RegisteredClaims
}

func generateStat() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func handleSpotifyLogic(c *fiber.Ctx) error {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")

	state := generateState()

	scope := "playlist-read-private playlist-read-collaborative playlist-modify-private playlist-modify-public user-read-private user-read-email"

	spotifyURL := fmt.Sprintf("https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s",
		clientID, url.QueryEscape(redirectURI), url.QueryEscape(scope), state)

	return c.JSON(fiber.Map{
		"auth_url": spotifyURL,
		"state":    state,
	})
}

func handleSpotifyCallback(c *fibe.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Authorization code not provided",
		})
	}

	tokenResp, err := exchangeCodeForTokens(code)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to exchange code for tokens",
		})
	}

	user, err := getSpotifyUserProfile(tokenRep.AccessToken)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Failed to get user profile",
		})
	}

}
