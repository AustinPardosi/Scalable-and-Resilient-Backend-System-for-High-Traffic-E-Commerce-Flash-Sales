package service

import (
	"os"
	"user-management-service/internal/entity"
	"user-management-service/internal/repository"

	"github.com/rs/zerolog"
)

var logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

type UserService struct {
	repo repository.UserRepository
}

// NewUserService creates a new user service
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// GetUserByID retrieves a user by their ID
func (s *UserService) GetUserByID(id int) (*entity.User, error) {
	user, err := s.repo.GetUserByID(id)
	if err != nil {
		logger.Error().Err(err).Msgf("Error retrieving user by ID %d", id)
		return nil, err
	}
	return user, nil
}

// CreateUser creates a new user
func (s *UserService) CreateUser(user *entity.User) (*entity.User, error) {
	user, err := s.repo.CreateUser(user)
	if err != nil {
		logger.Error().Err(err).Msg("Error creating user")
		return nil, err
	}
	return user, nil
}

// Login logs in a user with email and password
func (s *UserService) Login(email, password string) (*entity.User, error) {
	user, err := s.repo.GetUserByEmailAndPassword(email, password)
	if err != nil {
		logger.Error().Err(err).Msg("Error logging in user")
		return nil, err
	}
	return user, nil
}
