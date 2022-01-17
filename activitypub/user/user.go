package user

import (
	"fmt"
	"github.com/FediUni/FediUni/activitypub/actor"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost = 16
)

// User is used for authentication purposes.
type User struct {
	Username       string
	HashedPassword string
	Person         actor.Person
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
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password failed: got err=%v", err)
	}
	return &User{
		Username:       username,
		HashedPassword: string(hashedPassword),
		Person:         person,
	}, nil
}

func (u *User) BSON() ([]byte, error) {
	user, err := bson.Marshal(u)
	if err != nil {
		return nil, err
	}
	return user, nil
}
