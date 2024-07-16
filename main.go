package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

var db *sql.DB
var client = redis.NewClient(&redis.Options{
	Addr:         "localhost:6379",
	ReadTimeout:  2 * time.Second,
	WriteTimeout: 2 * time.Second,
})

type Products struct {
	ProductID   string  `json:"prodictId"`
	ProductName string  `json:"productName"`
	RetailPrice float64 `json:"retailPrice"`
}

func main() {
	r := gin.Default()

	srv := &http.Server{
		Addr:    ":" + os.Getenv("PORT"),
		Handler: r,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	connectDB()
	// err := client.Set(context.Background(), "Products", "", time.Second*10).Err() // ถ้าระบุเวลาเป็น 0 จะไม่มี expiration time
	// if err != nil {
	// 	fmt.Println("can set product to redis")
	// 	return
	// }
	r.GET("/api", getProduct)

	go gracefully(ctx, srv)

	if err := srv.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}

	fmt.Println("\nbye")
}

func gracefully(ctx context.Context, srv *http.Server) {
	<-ctx.Done()
	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}
}

func connectDB() {
	conndb, err := sql.Open("postgres", "postgresql://postgres:example@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		log.Fatal("Connect to database error", err)
	}

	fmt.Println("okay")
	db = conndb
}

func getProduct(ctx *gin.Context) {
	cmd := client.Get(context.Background(), "Products")
	result, err := cmd.Result()
	var res map[string]any
	if err != nil {
		rows, err := db.Query("SELECT product_id, product_name, retail_price FROM products")
		if err != nil {
			res = map[string]any{
				"status":  "Error",
				"message": "Can't get products",
			}
			fmt.Println(err)
			ctx.JSON(http.StatusNotFound, res)
			return
		} else {
			products := []Products{}
			for rows.Next() {
				var product = Products{}
				err := rows.Scan(&product.ProductID, &product.ProductName, &product.RetailPrice)
				if err != nil {
					fmt.Println("can't Scan row into variable", err)
					ctx.JSON(http.StatusInternalServerError, map[string]error{"message": err})
					return
				}
				products = append(products, product)
			}
			jsonData, errT := json.Marshal(products)
			if errT != nil {
				fmt.Println("Error marshaling map:", err)
				ctx.JSON(http.StatusInternalServerError, map[string]error{"message": errT})
				return
			}
			err := client.Set(context.Background(), "Products", jsonData, time.Second*10).Err() // ถ้าระบุเวลาเป็น 0 จะไม่มี expiration time
			if err != nil {
				fmt.Println("can set product to redis")
				ctx.JSON(http.StatusInternalServerError, map[string]error{"message": err})
				return
			}

			res = map[string]any{
				"status":   "Success",
				"location": "database",
				"data":     products,
			}
			ctx.JSON(http.StatusOK, res)
		}
	} else {
		m := []Products{}
		err := json.Unmarshal([]byte(result), &m)
		if err != nil {
			fmt.Println("Can't convert data")
			ctx.JSON(http.StatusInternalServerError, map[string]error{"message": err})
			return
		}
		res = map[string]any{
			"status":   "Success",
			"location": "redis",
			"data":     m,
		}
		ctx.JSON(http.StatusOK, res)
	}
}
