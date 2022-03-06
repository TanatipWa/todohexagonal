package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/time/rate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/tanatipwa/todos/auth"
	"github.com/tanatipwa/todos/router"
	"github.com/tanatipwa/todos/store"
	"github.com/tanatipwa/todos/todo"
)

var (
	buildcommit = "dev"
	buildtime   = time.Now().String()
)

func main() {
	// _, err := os.Create("/tmp/live")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer os.Remove("/tmp/live")

	err := godotenv.Load("local.env")
	if err != nil {
		log.Println("please consider environment variables: %s", err)
	}
	db, err := gorm.Open(sqlite.Open(os.Getenv("DB_CONN")), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.AutoMigrate(&todo.Todo{})

	// r := gin.Default()
	// config := cors.DefaultConfig()
	// config.AllowOrigins = []string{
	// 	"http://localhost:8080",
	// }
	// config.AllowHeaders = []string{
	// 	"Origin",
	// 	"Authorization",
	// 	"TranscationID",
	// }
	// r.Use(cors.New(config))

	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://mongoadmin:secret@localhost:27017"))
	if err != nil {
		panic("failed to connect database")
	}
	collection := client.Database("myapp").Collection("todos")

	r := router.NewMyRouter()
	r2 := router.NewFiberRouter()

	r.GET("/healthz", func(c *gin.Context) {
		c.Status(200)
	})
	r.GET("/limitz", limitedHandler)
	r.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"buildcommit": buildcommit,
			"buildtime":   buildtime,
		})
	})

	r.GET("/tokenz", auth.AccessToken(os.Getenv("SIGN")))

	protected := r.Group("", auth.Protect([]byte(os.Getenv("SIGN"))))

	// gormStore := store.NewGormStore(db)
	mongoStore := store.NewMongoDBStore(collection)

	handler := todo.NewTodoHandler(mongoStore)

	r2.POST("/todos", handler.NewTask) // Hex
	// protected.POST("/todos", router.NewGinHandler(handler.NewTask))
	protected.GET("/todos", handler.List)
	protected.DELETE("/todos/:id", handler.Remove)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &http.Server{
		Addr:           ":" + os.Getenv("PORT"),
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// fiber
	if err := r2.Listen(":" + os.Getenv("PORT")); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %s\n", err)
	}

	// gin
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()
	stop()
	fmt.Println("shutting down gracefully, press Ctrl+C agin to force")

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(timeoutCtx); err != nil {
		fmt.Println(err)
	}

	// r.Run()
}

var limiter = rate.NewLimiter(5, 5)

func limitedHandler(c *gin.Context) {
	if !limiter.Allow() {
		c.AbortWithStatus(http.StatusTooManyRequests)
		return
	}
	c.JSON(200, gin.H{
		"message": "pong",
	})
}
