package main

import (
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
	// Ініціалізація клієнта Redis
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // використовується адреса за замовчуванням
		DB:   0,                // використовується база даних за замовчуванням
	})

	// Перевірка з'єднання з Redis
	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Не вдалося підключитися до Redis: %v", err)
	}

	// Створення роутера
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/tasks", getTasks)       // Маршрут для отримання завдань
	r.Post("/tasks", createTask)    // Маршрут для створення завдання
	r.Put("/tasks/{id}", updateTask) // Маршрут для оновлення завдання
	r.Delete("/tasks/{id}", deleteTask) // Маршрут для видалення завдання

	fmt.Println("Сервер працює на порту 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Функція для отримання всіх завдань
func getTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // Використання контексту запиту

	val, err := rdb.Get(ctx, "tasks").Result()
	if err == redis.Nil {
		// Кеш відсутній, отримуємо дані з "бази даних"
		tasksMutex.Lock()
		taskList := make([]Task, 0, len(tasks))
		for _, task := range tasks {
			taskList = append(taskList, task)
		}
		tasksMutex.Unlock()

		// Збереження в кеш Redis
		jsonData, err := json.Marshal(taskList)
		if err != nil {
			http.Error(w, "Помилка кодування завдань: "+err.Error(), http.StatusInternalServerError)
			return
		}

		err = rdb.Set(ctx, "tasks", jsonData, time.Minute*10).Err()
		if err != nil {
			http.Error(w, "Помилка збереження в кеш Redis: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Повернення відповіді
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(jsonData)
		if err != nil {
			http.Error(w, "Помилка запису відповіді: "+err.Error(), http.StatusInternalServerError)
		}
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		// Кеш наявний, повертаємо кешовані дані
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(val))
		if err != nil {
			http.Error(w, "Помилка запису відповіді: "+err.Error(), http.StatusInternalServerError)
		}
	}
}

// Функція для створення нового завдання
func createTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // Використання контексту запиту

	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tasksMutex.Lock()
	tasks[task.ID] = task
	tasksMutex.Unlock()

	// Інвалідація кешу Redis
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Помилка видалення кешу Redis: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Функція для оновлення існуючого завдання
func updateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // Використання контексту запиту
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

	// Інвалідація кешу Redis
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Помилка видалення кешу Redis: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Функція для видалення існуючого завдання
func deleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context() // Використання контексту запиту
	id := chi.URLParam(r, "id")

	tasksMutex.Lock()
	delete(tasks, id)
	tasksMutex.Unlock()

	// Інвалідація кешу Redis
	err := rdb.Del(ctx, "tasks").Err()
	if err != nil {
		http.Error(w, "Помилка видалення кешу Redis: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

