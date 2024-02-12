package main

import (
	"fmt"

	"github.com/kvitvn/go-evt/pkg/event"
)

// User define a basic user definition for purpose
type User struct {
	Username string
	Email    string
}

// Other represents another struct for purpose
type Other struct {
	Data string
}

// UserListener define an event attach to User
type UserListener struct {
	name     string
	priority int
}

// NewUserListener return the user listener
func NewUserListener(name string, priority int) UserListener {
	return UserListener{
		name:     name,
		priority: priority,
	}
}

// Name return the name of the listener
func (ul UserListener) Name() string {
	return ul.name
}

// Priority return the event's priority
func (ul UserListener) Priority() int {
	return ul.priority
}

// Start check if data pass to the event corresponding to User
func (ul UserListener) Start(data interface{}) bool {
	if _, ok := data.(User); ok {
		return true
	}
	fmt.Printf("Data mismatch the expected type. Unable to process this listener\n")
	return false
}

// Process execute logics of the event
func (ul UserListener) Process(data interface{}) error {
	fmt.Printf("Processing UserListener with name %s and priority %d\n", ul.name, ul.priority)
	return nil
}

var (
	emitter = event.NewEmitter()
)

func init() {
	emitter.AddListener(NewUserListener("user.general", 1))
	emitter.AddListener(NewUserListener("user.general", 0))
}

func main() {
	user := User{
		Username: "foobar",
		Email:    "foobar@test.com",
	}

	emitter.Emit("user.general", user)

}
