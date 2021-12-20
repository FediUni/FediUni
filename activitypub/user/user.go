package user

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost = 16
)

// User is used for authentication purposes.
type User struct {
	Username       string
	HashedPassword string
}

// NewUser returns a valid user struct.
func NewUser(username, password string) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("username is unspecified: got username=%q", username)
	}
	if password == "" {
		return nil, fmt.Errorf("password is unspecified: got password=%q", password)
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password failed: got err=%v", err)
	}
	return &User{
		Username:       username,
		HashedPassword: string(hashedPassword),
	}, nil
}
