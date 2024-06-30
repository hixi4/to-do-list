package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Структура для завдання
type Task struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
}

// Збереження списку завдань у пам'яті
var (
	tasks  = []Task{}
	nextID = 1
	mutex  sync.Mutex
)

// Обробник для отримання списку завдань
func getTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mutex.Lock()
	defer mutex.Unlock()
	json.NewEncoder(w).Encode(tasks)
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
	tasks = append(tasks, task)
	mutex.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// Обробник для зміни існуючого завдання
func updateTask(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/tasks/")
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
	for i, task := range tasks {
		if task.ID == id {
			tasks[i].Name = updatedTask.Name
			tasks[i].Completed = updatedTask.Completed
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tasks[i])
			return
		}
	}
	http.Error(w, "Task not found", http.StatusNotFound)
}

// Обробник для видалення завдання
func deleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/tasks/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()
	for i, task := range tasks {
		if task.ID == id {
			tasks = append(tasks[:i], tasks[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	http.Error(w, "Task not found", http.StatusNotFound)
}

func main() {
	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getTasks(w, r)
		case http.MethodPost:
			addTask(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("Сервер працює на порту %d\n", listener.Addr().(*net.TCPAddr).Port)
	log.Fatal(http.Serve(listener, nil))
}
