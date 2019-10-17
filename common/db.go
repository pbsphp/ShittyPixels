package common

import (
	"encoding/json"
	"github.com/go-redis/redis"
)

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
