package repository

import "user-management-service/internal/entity"

type UserRepository struct {
}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

var users = map[int]*entity.User{
	1: {ID: 1, Username: "John Doe", Email: "john.doe@example.com"},
	2: {ID: 2, Username: "Jane Smith", Email: "jane.smith@example.com"},
}

func (r *UserRepository) GetUserByID(id int) (*entity.User, error) {
	user, exists := users[id]
	if !exists {
		return nil, nil
	}
	return user, nil
}

func (r *UserRepository) CreateUser(user *entity.User) (*entity.User, error) {
	user.ID = len(users) + 1
	users[user.ID] = user
	return user, nil
}

func (r *UserRepository) GetUserByEmail(email string) (*entity.User, error) {
	for _, user := range users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, nil
}

func (r *UserRepository) GetUserByEmailAndPassword(email, password string) (*entity.User, error) {
	for _, user := range users {
		if user.Email == email && user.Password == password {
			return user, nil
		}
	}
	return nil, nil
}
