package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Demo login API — Storyblocks jaisa flow simulate karta hai (local testing only).
// Valid accounts:
//   test@demo.com : password123
//   admin@demo.com : admin123
//   user@storyblocks.demo : demo2024

var validAccounts = map[string]string{
	"test@demo.com":         "password123",
	"admin@demo.com":        "admin123",
	"user@storyblocks.demo": "demo2024",
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Success     bool   `json:"success"`
	AccessToken string `json:"access_token,omitempty"`
	Plan        string `json:"plan,omitempty"`
	Error       string `json:"error,omitempty"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/auth/login", handleLogin)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	fmt.Println("========================================")
	fmt.Println("  Demo Login Server")
	fmt.Println("  http://localhost:3000")
	fmt.Println("----------------------------------------")
	fmt.Println("  Valid test accounts:")
	for email, pass := range validAccounts {
		fmt.Printf("    %s : %s\n", email, pass)
	}
	fmt.Println("========================================")

	if err := http.ListenAndServe(":3000", mux); err != nil {
		fmt.Println("error:", err)
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, loginResponse{Success: false, Error: "invalid request"})
		return
	}

	expected, ok := validAccounts[req.Email]
	if !ok || expected != req.Password {
		writeJSON(w, 401, loginResponse{Success: false, Error: "invalid credentials"})
		return
	}

	plan := "basic"
	if req.Email == "admin@demo.com" {
		plan = "enterprise"
	}

	writeJSON(w, 200, loginResponse{
		Success:     true,
		AccessToken: "demo_token_" + req.Email,
		Plan:        plan,
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
