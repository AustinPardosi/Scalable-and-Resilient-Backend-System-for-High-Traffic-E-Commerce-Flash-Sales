package repository

import (
	"context"
	"database/sql"
	"errors"
	"user-management-service/internal/entity"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrNotFound  = errors.New("user not found")
	ErrDuplicate = errors.New("email already registered")
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *entity.User) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash) VALUES (?, ?, ?)`,
		user.Username, user.Email, user.PasswordHash)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 { // duplicate key on email

		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	user.ID, err = res.LastInsertId()
	return err
}

func (r *UserRepository) GetUserByID(ctx context.Context, id int64) (*entity.User, error) {
	return r.getOne(ctx, `SELECT id, username, email, password_hash FROM users WHERE id = ?`, id)
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*entity.User, error) {
	return r.getOne(ctx, `SELECT id, username, email, password_hash FROM users WHERE email = ?`, email)
}

func (r *UserRepository) getOne(ctx context.Context, query string, arg any) (*entity.User, error) {
	var u entity.User
	err := r.db.QueryRowContext(ctx, query, arg).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
