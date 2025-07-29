package service

import "user-management-service/internal/entity"

type UserService struct {
}

// NewUserService creates a new user service
func NewUserService() *UserService {
	return &UserService{}
}

// GetUserByID retrieves a user by their ID
func (s *UserService) GetUserByID(id int) (*entity.User, error) {
	user := &entity.User{
		ID:       id,
		Username: "testuser",
		Email:    "email@example.com",
	}
	return user, nil
}

// CreateUser creates a new user
func (s *UserService) CreateUser(user *entity.User) (*entity.User, error) {
	user.ID = 1
	return user, nil
}