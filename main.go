package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/gofiber/fiber/v2/middleware/timeout"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
	"time"
)

// Initialize a connection pool
func initializeDbConnectionPool() (*pgxpool.Pool, error) {
	connString := fmt.Sprintf("postgres://%s:%s@postgresql:%s/%s?sslmode=disable", os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_PORT"), os.Getenv("POSTGRES_DB"))
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %v", err)
	}

	return pool, nil
}

func initializeRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("redis:%s", os.Getenv("REDIS_PORT")),
	})
}
func main() {
	redisClient := initializeRedisClient()
	// urlExample := "postgres://username:password@localhost:5432/database_name"
	dbpool, err := initializeDbConnectionPool()
	if err != nil {
		log.Fatal(os.Stderr, "Unable to create connection pool: %v\n", err)
		return
	}
	defer dbpool.Close()

	app := fiber.New(fiber.Config{
		Prefork:       true,
		CaseSensitive: true,
		StrictRouting: true,
		ServerHeader:  "Fiber",
		AppName:       "Test App v1.0.1",
	})

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("dbPool", dbpool)
		// Go to next middleware:
		return c.Next()
	})
	handler := func(ctx *fiber.Ctx) error {
		table := ctx.Params("table")
		level1 := ctx.Params("level")
		// SAVE TO DATABASE BEFORE
		dbPool := ctx.Locals("dbPool").(*pgxpool.Pool)
		tx, err := dbPool.Begin(ctx.UserContext())
		if err != nil {
			return ctx.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer func() {
			if err != nil {
				tx.Rollback(ctx.UserContext())
				log.Error("Transaction rolled back due to error:", err)
			}
		}()
		_, err = tx.Exec(ctx.UserContext(), "INSERT INTO matching_games (user_id, level, table_g) VALUES (1, $1, $2)", level1, table)
		if err != nil {
			return ctx.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		err = tx.Commit(ctx.UserContext())
		if err != nil {
			return ctx.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if err := redisClient.Publish(ctx.UserContext(), fmt.Sprintf("user-table-%s-level-%s", table, level1), 1).Err(); err != nil {
			log.Fatal("Error publishing message to Redis: ", err)
			return ctx.Status(500).JSON(fiber.Map{"error": "Error publishing message to Redis"})
		}

		i, err := strconv.Atoi(level1)
		if err != nil {
			log.Fatal("level must be an integer")
			return ctx.Status(400).JSON(fiber.Map{"error": "level must be an integer"})
		}
		level2 := strconv.Itoa(i + 1)

		subscriber := redisClient.Subscribe(ctx.UserContext(), fmt.Sprintf("user-table-%s-level-%s", table, level1), fmt.Sprintf("user-table-%s-level-%s", table, level2))
		for {
			msg, err := subscriber.ReceiveMessage(ctx.UserContext())
			if err != nil {
				return ctx.Status(500).JSON(fiber.Map{"error": err.Error()})
			}
			log.Info("Received message from " + msg.Channel + " channel. Payload: " + msg.Payload)
			return ctx.Status(200).JSON(fiber.Map{"message": fmt.Sprintf("Found match with user %s", msg.Payload)})
		}
	}
	app.Get("/:table/:level", timeout.NewWithContext(handler, 60*time.Second))

	err = app.Listen(":3000")
	if err != nil {
		return
	}
}
