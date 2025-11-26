package main

import (
	"context"
	"fmt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/app"
)

func main() {
	dsn := "postgres://admin:MeetuperPassword@localhost:5432/postgres?sslmode=disable"
	application := app.New(dsn)

	err := application.Start(context.Background())
	if err != nil {
		fmt.Println("failed to start app:", err)
	}
}
