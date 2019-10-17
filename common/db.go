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

package common

import (
	"encoding/json"
	"github.com/go-redis/redis"
	"strconv"
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

// Load data from redis.
func RedisLoad(rdb *redis.Client, entity string, key string, rec interface{}) error {
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

// Store data into redis.
func RedisStore(rdb *redis.Client, entity string, key string, rec interface{}) error {
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

func GetUserByLogin(rdb *redis.Client, login string) (*UserData, error) {
	var rec UserData
	err := RedisLoad(rdb, "User", login, &rec)
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func StoreUser(rdb *redis.Client, user *UserData) error {
	return RedisStore(rdb, "User", user.Login, user)
}

func GetSessionBySessionId(rdb *redis.Client, sessionId string) (*SessionData, error) {
	var rec SessionData
	err := RedisLoad(rdb, "Session", sessionId, &rec)
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rec, nil
}

func StoreSession(rdb *redis.Client, session *SessionData) error {
	return RedisStore(rdb, "Session", session.Id, session)
}

// Check cooldown for user session.
// Return true if there was cooldown info (user made request less than `CooldownSeconds' seconds ago).
// Otherwise return false AND add cooldown info.
// Should be atomic.
func TestAndUpdateSessionCooldown(rdb *redis.Client, appConfig *AppConfig, sessionId string) (error, bool) {
	// Unfortunately, GETSET command has no TTL. Also there is no test-and-set command.
	// So we do this:
	// x = GETSET token, expireTime      # get old cooldown record and store new one.
	// if x is present and x > now:      # there was cooldown info. User is too fast.
	//   SET token x 					 # Set old cooldown info back.
	key := "Cooldown:" + sessionId
	currentTime := time.Now().Unix()
	cooldownSec := int64(appConfig.CooldownSeconds)
	cooldownAsTime := time.Duration(cooldownSec) * time.Second
	expirySec := currentTime + cooldownSec

	oldExpiryStr, err := rdb.GetSet(key, expirySec).Result()
	if err != nil && err != redis.Nil {
		return err, false
	}

	if err == nil {
		if oldExpiry, err := strconv.ParseInt(oldExpiryStr, 10, 64); err == nil {
			if oldExpiry > currentTime {
				if err := rdb.Set(key, oldExpiryStr, cooldownAsTime).Err(); err != nil {
					return err, false
				}
				return nil, true
			}
		}
	}

	// Set TTL to avoid outdated records.
	if err := rdb.Expire(key, cooldownAsTime).Err(); err != nil {
		return err, false
	}

	return nil, false
}

func GetSessionCooldownBySessionId(rdb *redis.Client, sessionId string) (int, error) {
	key := "Cooldown:" + sessionId
	currentTime := time.Now().Unix()

	expiryStr, err := rdb.Get(key).Result()
	if err != nil && err != redis.Nil {
		return 0, err
	}
	if err == redis.Nil {
		return 0, nil
	}
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return 0, nil
	}
	cooldown := expiry - currentTime
	if cooldown < 0 {
		cooldown = 0
	}

	return int(cooldown), nil
}
