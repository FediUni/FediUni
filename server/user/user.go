package user

import (
	"fmt"
	"github.com/FediUni/FediUni/server/actor"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost = 10
)

// User is used for authentication purposes.
type User struct {
	Username string `json:"username" bson:"username"`
	Password string `json:"password" bson:"password"`
	Person   actor.Person
}

// NewUser returns a valid user struct.
func NewUser(username, password string, person actor.Person) (*User, error) {
	if username == "" {
		return nil, fmt.Errorf("username is unspecified: got username=%q", username)
	}
	if person == nil {
		return nil, fmt.Errorf("person is not initialized: got %v", nil)
	}
	if password == "" {
		return nil, fmt.Errorf("password is unspecified: got password=%q", password)
	}
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password failed: got err=%v", err)
	}
	return &User{
		Username: username,
		Password: string(hashedPassword),
		Person:   person,
	}, nil
}

func HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
}
