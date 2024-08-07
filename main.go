package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-redis/redis/v8"
)

var rdb *redis.Client
var tasksMutex sync.Mutex

type Task struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

var tasks = make(map[string]Task)

func main() {
	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // use default Addr
		DB:   0,                // use default DB
	})

	// Check Redis connection
	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/tasks", getTasks)
	r.Post("/tasks", createTask)
	r.Put("/tasks/{id}", updateTask)
	r.Delete("/tasks/{id}", deleteTask)

	fmt.Println("Server is running on port 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	val, err := rdb.Get(ctx, "tasks").Result()
	if err == redis.Nil {
		// Cache miss, fetch from "database"
		tasksMutex.Lock()
		taskList := make([]Task, 0, len(tasks))
		for _, task := range tasks {
			taskList = append(taskList, task)
		}
		tasksMutex.Unlock()

		// Store in Redis cache
		jsonData, err := json.Marshal(taskList)
		if err != nil {
			http.Error(w, "Error encoding tasks: "+err.Error(), http.StatusInternalServerError)
			return
		}

		err = rdb.Set(ctx, "tasks", jsonData, time.Minute*10).Err()
		if err != nil {
			http.Error(w, "Error setting Redis cache: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(jsonData)
		if err != nil {
			http.Error(w, "Error writing response: "+err.Error(), http.StatusInternalServerError)
		}
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		// Cache hit, return cached data
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(val))
		if err != nil {
			http.Error(w, "Error writing response: "+err.Error(), http.StatusInternalServerError)
		}
	}
}

func createTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tasksMutex.Lock()
	tasks[task.ID] = task
	tasksMutex.Unlock()

	// Invalidate Redis cache
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Error deleting Redis cache: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func updateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	task.ID = id

	tasksMutex.Lock()
	tasks[id] = task
	tasksMutex.Unlock()

	// Invalidate Redis cache
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Error deleting Redis cache: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	tasksMutex.Lock()
	delete(tasks, id)
	tasksMutex.Unlock()

	// Invalidate Redis cache
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Error deleting Redis cache: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}


