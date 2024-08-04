package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// Контекст для Redis
var ctx = context.Background()

// Клієнт Redis
var rdb *redis.Client

// Структура для завдання
type Task struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
}

// Збереження списку завдань у пам'яті
var (
	tasks  = map[int]Task{}
	nextID = 1
	mutex  sync.Mutex
)

// Обробник для отримання списку завдань
func getTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mutex.Lock()
	defer mutex.Unlock()

	// Перевірка кешу Redis
	val, err := rdb.Get(ctx, "tasks").Result()
	if err == redis.Nil {
		// Якщо кеш відсутній, отримати з "бази даних" в пам'яті
		taskList := make([]Task, 0, len(tasks))
		for _, task := range tasks {
			taskList = append(taskList, task)
		}

		// Зберегти в кеш Redis
		jsonData, _ := json.Marshal(taskList)
		rdb.Set(ctx, "tasks", jsonData, time.Minute*10)

		// Повернути відповідь
		w.Write(jsonData)
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		// Якщо кеш присутній, повернути кешовані дані
		w.Write([]byte(val))
	}
}

// Обробник для додавання нового завдання
func addTask(w http.ResponseWriter, r *http.Request) {
	var task Task
	err := json.NewDecoder(r.Body).Decode(&task)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	mutex.Lock()
	task.ID = nextID
	nextID++
	tasks[task.ID] = task
	mutex.Unlock()

	// Інвалювати кеш Redis
	rdb.Del(ctx, "tasks")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// Обробник для зміни існуючого завдання
func updateTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/tasks/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	var updatedTask Task
	err = json.NewDecoder(r.Body).Decode(&updatedTask)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()
	if task, exists := tasks[id]; exists {
		task.Name = updatedTask.Name
		task.Completed = updatedTask.Completed
		tasks[id] = task

		// Інвалювати кеш Redis
		rdb.Del(ctx, "tasks")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task)
		return
	}

	http.Error(w, "Task not found", http.StatusNotFound)
}

// Обробник для видалення завдання
func deleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/tasks/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()
	if _, exists := tasks[id]; exists {
		delete(tasks, id)

		// Інвалювати кеш Redis
		rdb.Del(ctx, "tasks")

		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Error(w, "Task not found", http.StatusNotFound)
}

func main() {
	// Ініціалізація клієнта Redis
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // використовується адреса за замовчуванням
		DB:   0,                // використовується БД за замовчуванням
	})

	// Перевірка підключення до Redis
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Не вдалося підключитися до Redis: %v", err)
	}

	// Ініціалізація маршрутизатора
	mux := http.NewServeMux()

	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getTasks(w, r)
		case http.MethodPost:
			addTask(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			updateTask(w, r)
		case http.MethodDelete:
			deleteTask(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Слухаємо на динамічному порту
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	log.Printf("Сервер працює на порту %d\n", port)

	log.Fatal(http.Serve(listener, mux))
}
