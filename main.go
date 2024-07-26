package main

import (
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"os"
	"strconv"
	"time"
)

type MatchmakingRequest struct {
	PlayerID int
	Level    int
	Table    int
}

var rdb *redis.Client

func enqueueMatchmakingRequest(ctx *fiber.Ctx, req MatchmakingRequest) error {
	playerKey := fmt.Sprintf("player:%d", req.PlayerID)
	tableKey := "matchmaking:table"
	// Store player parameters in a hash
	err := rdb.HSet(ctx.UserContext(), playerKey, "Level", req.Level, "Table", req.Table).Err()
	if err != nil {
		return fmt.Errorf("failed to store player parameters: %v", err)
	}
	rdb.Expire(ctx.UserContext(), playerKey, 10*time.Second)
	// Add player to the sorted set by level
	err = rdb.ZAdd(ctx.UserContext(), tableKey, &redis.Z{
		Score:  float64(req.Level),
		Member: req.PlayerID,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to add player to matchmaking queue: %v", err)
	}

	return nil
}

func initializeRedisClient() {
	rdb = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("redis:%s", os.Getenv("REDIS_PORT")),
	})
}

func findMatch(ctx *fiber.Ctx, req MatchmakingRequest, minPlayers, maxPlayers int, timeout time.Duration) ([]int, error) {
	tableKey := "matchmaking:table"
	start := time.Now()
	for {
		// Check for timeout
		if time.Since(start) > timeout {
			// Handle fallback or return error
			return nil, fmt.Errorf("timeout reached, no match found")
		}

		// Find potential matches
		results, err := rdb.ZRangeByScore(ctx.UserContext(), tableKey, &redis.ZRangeBy{
			Min: strconv.Itoa(req.Level - 1),
			Max: strconv.Itoa(req.Level + 1),
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to search for matches: %v", err)
		}

		// Filter results based on additional conditions
		var matches []int
		for _, res := range results {
			playerID, _ := strconv.Atoi(res)
			playerParams, err := rdb.HGetAll(ctx.UserContext(), fmt.Sprintf("player:%d", playerID)).Result()
			if err != nil {
				continue
			}

			table, _ := strconv.Atoi(playerParams["Table"])
			log.Debug("results", table, req.Table, playerID, req.PlayerID, table == req.Table && playerID != req.PlayerID)
			if table == req.Table && playerID != req.PlayerID {
				matches = append(matches, playerID)
			}
		}
		log.Info(len(matches), minPlayers)
		// Check if we have enough players for a match
		if len(matches) >= minPlayers-1 {
			return matches, nil
		}

		time.Sleep(200 * time.Millisecond) // Avoid busy waiting
	}
}

func main() {
	initializeRedisClient()

	app := fiber.New(fiber.Config{
		Prefork:       true,
		CaseSensitive: true,
		StrictRouting: true,
		ServerHeader:  "Fiber",
		AppName:       "Test App v1.0.1",
	})
	app.Use(recover.New())
	handler := func(ctx *fiber.Ctx) error {
		table := ctx.Params("table")
		level := ctx.Params("level")
		reqHeaders := ctx.GetReqHeaders()
		playerID := reqHeaders["Playerid"][0]

		iLevel, err := strconv.Atoi(level)
		if err != nil {
			log.Fatal("level must be an integer")
			return ctx.Status(400).JSON(fiber.Map{"error": "level must be an integer"})
		}
		iTable, err := strconv.Atoi(table)
		if err != nil {
			log.Fatal("level must be an integer")
			return ctx.Status(400).JSON(fiber.Map{"error": "table must be an integer"})
		}
		iPlayerID, err := strconv.Atoi(playerID)
		if err != nil {
			log.Fatal("level must be an integer")
			return ctx.Status(400).JSON(fiber.Map{"error": "table must be an integer"})
		}

		req := MatchmakingRequest{
			PlayerID: iPlayerID,
			Level:    iLevel,
			Table:    iTable,
		}
		err = enqueueMatchmakingRequest(ctx, req)
		if err != nil {
			log.Fatalf("Error enqueuing matchmaking request: %v", err)
		}
		minPlayers := 2
		maxPlayers := 4
		timeoutCheck := 10 * time.Second

		matches, err := findMatch(ctx, req, minPlayers, maxPlayers, timeoutCheck)
		if err != nil {
			return ctx.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Error finding match: %v", err)})
		}
		log.Info("Match found with players: %v\n", matches)
		return ctx.Status(200).JSON(fiber.Map{"message": fmt.Sprintf("Match found with players: %v", matches)})
	}

	app.Get("/:table/:level", handler)

	err := app.Listen(":3000")
	if err != nil {
		return
	}
}
