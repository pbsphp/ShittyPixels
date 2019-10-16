/*
   ShittyPixels
   Copyright © 2019  Pbsphp

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"encoding/json"
	"github.com/go-redis/redis"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type UserData struct {
	Login        string
	PasswordHash string
}

type SessionData struct {
	Login            string
	Id               string
	ValidationErrors map[string]string
}

var templates = template.Must(
	template.ParseFiles(
		"templates/index.html",
		"templates/register.html",
		"templates/login.html",
		"templates/canvas.html",
	),
)

func renderTemplate(w http.ResponseWriter, tmpl string, p interface{}) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func redisLoad(rdb *redis.Client, entity string, key string, rec interface{}) error {
	rawVal, err := rdb.Get(entity + ":" + key).Result()
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(rawVal), &rec)
	if err != nil {
		return err
	}

	return nil
}

func redisStore(rdb *redis.Client, entity string, key string, rec interface{}) error {
	rawVal, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	err = rdb.Set(entity+":"+key, rawVal, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func getUserByLogin(rdb *redis.Client, login string) (*UserData, error) {
	var rec UserData
	err := redisLoad(rdb, "User", login, &rec)
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func storeUser(rdb *redis.Client, user *UserData) error {
	return redisStore(rdb, "User", user.Login, user)
}

func getSessionBySessionId(rdb *redis.Client, sessionId string) (*SessionData, error) {
	var rec SessionData
	err := redisLoad(rdb, "Session", sessionId, &rec)
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func storeSession(rdb *redis.Client, session *SessionData) error {
	return redisStore(rdb, "Session", session.Id, session)
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func generateSessionToken() string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	tokenLength := 32

	b := make([]rune, tokenLength)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func indexHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData) {
	context := struct {
		User string
	}{
		User: session.Login,
	}

	renderTemplate(w, "index", &context)
}

func registerHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData) {
	if session.Login != "" {
		http.Redirect(w, r, "/canvas", 302)
	}

	if r.Method == "POST" {
		login := r.FormValue("login")
		password := r.FormValue("password")

		validationErrors := make(map[string]string)
		isValid := true

		user, err := getUserByLogin(rdb, login)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user != nil {
			validationErrors["login"] = "User already exists"
			isValid = false
		}
		if login == "" {
			validationErrors["login"] = "Login is empty"
			isValid = false
		}
		if password == "" {
			validationErrors["password"] = "Password is empty"
			isValid = false
		}

		if isValid {
			passHash, err := hashPassword(password)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			err = storeUser(rdb, &UserData{
				Login:        login,
				PasswordHash: passHash,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/login", 302)
		} else {
			session.ValidationErrors = validationErrors
			http.Redirect(w, r, "/register", 302)
		}
	} else {
		renderTemplate(w, "register", &struct {
			ValidationErrors map[string]string
		}{
			ValidationErrors: session.ValidationErrors,
		})
		session.ValidationErrors = make(map[string]string)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData) {
	if session.Login != "" {
		http.Redirect(w, r, "/canvas", 302)
	}

	if r.Method == "POST" {
		login := r.FormValue("login")
		password := r.FormValue("password")

		validationErrors := make(map[string]string)
		isValid := true

		user, err := getUserByLogin(rdb, login)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if user == nil {
			validationErrors["login"] = "User not found"
			isValid = false
		}
		if login == "" {
			validationErrors["login"] = "Login is empty"
			isValid = false
		}
		if password == "" {
			validationErrors["password"] = "Password is empty"
			isValid = false
		}
		if user != nil && !checkPasswordHash(password, user.PasswordHash) {
			validationErrors["password"] = "Wrong password"
			isValid = false
		}

		if user != nil && isValid {
			session.Login = user.Login
			http.Redirect(w, r, "/canvas", 302)
		} else {
			session.ValidationErrors = validationErrors
			http.Redirect(w, r, "/login", 302)
		}
	} else {
		renderTemplate(w, "login", &struct {
			ValidationErrors map[string]string
		}{
			ValidationErrors: session.ValidationErrors,
		})
		session.ValidationErrors = make(map[string]string)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData) {
	session.Login = ""
	session.ValidationErrors = map[string]string{}
	http.Redirect(w, r, "/", 302)
}

func canvasHandler(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData) {
	if session.Login == "" {
		http.Redirect(w, r, "/login", 302)
	}

	renderTemplate(w, "canvas", nil)
}

func makeHandler(
	fn func(w http.ResponseWriter, r *http.Request, rdb *redis.Client, session *SessionData),
	rdb *redis.Client,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("sessionId")
		if err != nil && err != http.ErrNoCookie {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var session *SessionData
		if cookie != nil {
			session, err = getSessionBySessionId(rdb, cookie.Value)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if session == nil {
			session = &SessionData{
				Login:            "",
				Id:               generateSessionToken(),
				ValidationErrors: make(map[string]string),
			}
			http.SetCookie(w, &http.Cookie{
				Name:  "sessionId",
				Value: session.Id,
				Path:  "/",
			})
		}

		fn(w, r, rdb, session)

		err = storeSession(rdb, session)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	http.HandleFunc("/", makeHandler(indexHandler, rdb))
	http.HandleFunc("/register", makeHandler(registerHandler, rdb))
	http.HandleFunc("/login", makeHandler(loginHandler, rdb))
	http.HandleFunc("/logout", makeHandler(logoutHandler, rdb))
	http.HandleFunc("/canvas", makeHandler(canvasHandler, rdb))

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
