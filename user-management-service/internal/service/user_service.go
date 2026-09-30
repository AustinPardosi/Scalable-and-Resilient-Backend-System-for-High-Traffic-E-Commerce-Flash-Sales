package service

import (
	"context"
	"errors"
	"user-management-service/internal/entity"
	"user-management-service/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

// dummyHash is checked when the email is unknown, so both kinds of failed login take
// the same bcrypt time and response timing doesn't reveal which emails are registered.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

type UserService struct {
	repo *repository.UserRepository
}

// NewUserService creates a new user service
func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// GetUserByID retrieves a user by their ID
func (s *UserService) GetUserByID(ctx context.Context, id int64) (*entity.User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// CreateUser registers a user, storing only a bcrypt hash of the password
func (s *UserService) CreateUser(ctx context.Context, username, email, password string) (*entity.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &entity.User{Username: username, Email: email, PasswordHash: hash}
	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login checks email and password; unknown email and wrong password look the same to the caller
func (s *UserService) Login(ctx context.Context, email, password string) (*entity.User, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}
