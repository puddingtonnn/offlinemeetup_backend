package main

import (
	"context"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/app"
)

func main() {
	dsn := "postgres://admin:MeetuperPassword@localhost:5432/postgres?sslmode=disable"
	app := app.New(dsn)

	err := app.Start(context.TODO())
	if err != nil {
		fmt.Println("failed to start app:", err)
	}
}
