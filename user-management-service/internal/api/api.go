package api

import (
	"errors"
	"net/mail"
	"strconv"
	"time"
	"user-management-service/internal/repository"
	"user-management-service/internal/service"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type UserHandler struct {
	userService *service.UserService
	jwtSecret   []byte
}

func NewUserHandler(userService *service.UserService, jwtSecret []byte) *UserHandler {
	return &UserHandler{userService: userService, jwtSecret: jwtSecret}
}

type JwtCustomClaims struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

type credentials struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Me returns the caller's own profile --> /users/me
func (h *UserHandler) Me(c echo.Context) error {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok {
		return c.JSON(401, map[string]string{"error": "Unauthorized"})
	}
	sub, _ := token.Claims.GetSubject()
	userID, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return c.JSON(401, map[string]string{"error": "Unauthorized"})
	}
	user, err := h.userService.GetUserByID(c.Request().Context(), userID)
	if errors.Is(err, repository.ErrNotFound) {
		return c.JSON(404, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return internalError(c, err)
	}
	return c.JSON(200, user)
}

// CreateUser registers a new user --> /users
func (h *UserHandler) CreateUser(c echo.Context) error {
	var req credentials
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}
	// Plain addresses only: ParseAddress also accepts `Name <a@b.c>`, which we'd store verbatim.
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email || len(req.Email) > 255 {
		return c.JSON(400, map[string]string{"error": "a valid email of at most 255 characters is required"})
	}
	if req.Username == "" || len(req.Username) > 100 {
		return c.JSON(400, map[string]string{"error": "username must be 1-100 characters"})
	}
	// bcrypt only reads the first 72 bytes
	if len(req.Password) < 8 || len(req.Password) > 72 {
		return c.JSON(400, map[string]string{"error": "password must be 8-72 characters"})
	}
	newUser, err := h.userService.CreateUser(c.Request().Context(), req.Username, req.Email, req.Password)
	if errors.Is(err, repository.ErrDuplicate) {
		return c.JSON(409, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return internalError(c, err)
	}
	return c.JSON(201, newUser)
}

// Login exchanges email and password for a JWT --> /login
func (h *UserHandler) Login(c echo.Context) error {
	var req credentials
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request payload"})
	}

	user, err := h.userService.Login(c.Request().Context(), req.Email, req.Password)
	if errors.Is(err, service.ErrInvalidCredentials) {
		return c.JSON(401, map[string]string{"error": err.Error()})
	}
	if err != nil {
		return internalError(c, err)
	}

	claims := JwtCustomClaims{
		Name:  user.Username,
		Email: user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(user.ID, 10),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return internalError(c, err)
	}

	return c.JSON(200, map[string]string{"token": tokenString})
}

func internalError(c echo.Context, err error) error {
	log.Error().Err(err).Str("path", c.Path()).Msg("request failed")
	return c.JSON(500, map[string]string{"error": "internal error"})
}
