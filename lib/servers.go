package lib

import (
	"encoding/json"
	"github.com/go-redis/redis"
	"strconv"
	"strings"
	"time"
)

// redisServer 配置
type redisServer struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Auth   string `json:"auth"`
	DBNums int    `json:"dbNums"`
}

var maxServerID int

// QueryServers 获取列表
func QueryServers() GlobalConfigStruct {
	return globalConfigs
}

func QueryCli(serverID int, db int) (*redis.Client, error) {
	r, err := getClient(serverID, db)
	if err != nil {
		return r, err
	}
	return r, nil
}

func TestCli(name, address, password, port string) error {
	err := testClient(name, address, password, port)
	if err != nil {
		return err
	}
	return nil
}

func SaveCli(name, address, password, port string) error {
	timeObj := time.Now()
	unixtime := timeObj.Unix()

	p, _ := strconv.Atoi(port)
	r := redisServer{int(unixtime), name, address, p, password, 15}
	globalConfigs.Servers = append(globalConfigs.Servers, r)

	err := saveConf()
	if err != nil {
		return err
	}
	return nil
}

func SystemConfigs(data string) error {
	var s systemConf

	err := json.Unmarshal([]byte(data), &s)
	if err != nil {
		return err
	}
	globalConfigs.System = s
	err = saveConf()
	if err != nil {
		return err
	}
	return nil
}

func CloseCli(serverID int) error {
	err := closeClient(serverID)
	if err != nil {
		return err
	}
	return nil
}

func QueryScan(r *redis.Client, iterate int, match string) (interface{}, error) {
	if match == "" {
		match = "*"
	} else {
		match = match + "*"
	}
	ret, err := r.Do("scan", iterate, "MATCH", match, "COUNT", globalConfigs.System.KeyScanLimits).Result()
	if err != nil {
		return nil, err
	}
	if v, ok := ret.([]interface{}); ok {
		return map[string]interface{}{
			"iterate": v[0],
			"keys":    v[1],
		}, nil
	} else {
		return nil, err
	}
}

func QueryKey(r *redis.Client, key string, cursor uint64, match string) (rst RedisValue, err error) {
	rst.Key = key
	rst.T, rst.TTL, err = keyTypeTtl(r, key)
	if err != nil {
		return rst, err
	}

	switch rst.T {
	case "string":
		val, err := r.Get(key).Result()
		bel := strconv.Quote(val)
		bit := strings.Contains(bel, "\\x")
		b := strings.Contains(bel, "\\\\x")
		b2 := strings.Contains(bel, "\\b")
		if err != nil {
			return rst, err
		} else {
			if (bit && !b) || b2 {
				rst.Val = stringHandle([]byte(val))
				rst.Bit = true
			} else {
				rst.Val = val
			}
		}
	case "list":
		l, err := r.LLen(key).Result()
		if err != nil {
			return rst, err
		}

		lSet := globalConfigs.System.RowScanLimits
		list, err := r.LRange(key, int64(cursor), int64(lSet+int(cursor))).Result()
		if err != nil {
			return rst, err
		}
		rst.Iterate = lSet + int(cursor) + 1
		if rst.Iterate > int(l) {
			rst.Iterate = 0
		}
		rst.Size = int(l)
		rst.Val = formatSetAndList(list)
	case "set":
		ret, i, err := r.SScan(key, cursor, match, int64(globalConfigs.System.KeyScanLimits)).Result()
		if err != nil {
			return rst, err
		}

		l, err := r.SCard(key).Result()
		if err != nil {
			return rst, err
		}
		rst.Size = int(l)
		rst.Iterate = int(i)
		rst.Val = formatSetAndList(ret)
	case "zset":
		ret, i, err := r.ZScan(key, cursor, match, int64(globalConfigs.System.KeyScanLimits)).Result()
		if err != nil {
			return rst, err
		}

		l, err := r.ZCount(key, "-inf", "+inf").Result()
		if err != nil {
			return rst, err
		}
		rst.Size = int(l)
		rst.Iterate = int(i)
		rst.Val = formatZset(ret)
	case "hash":
		ret, i, err := r.HScan(key, cursor, match, int64(globalConfigs.System.KeyScanLimits)).Result()
		if err != nil {
			return rst, err
		}

		l, err := r.HLen(key).Result()
		if err != nil {
			return rst, err
		}
		rst.Size = int(l)
		rst.Iterate = int(i)
		rst.Val = formatHash(ret)
	}

	return rst, nil
}

func UpdateValue(valOrignal, fieldOrignal, val, key, field, t string, index int64, r *redis.Client) error {
	switch t {
	case "string":
		t, err := r.TTL(key).Result()
		if err != nil {
			return err
		}
		if fieldOrignal != "bit" {
			_, err = r.Set(key, val, t).Result()
		}
		if err != nil {
			return err
		}
	case "set":
		_, err := r.SAdd(key, val).Result()
		if err != nil {
			return err
		}
		_, err = r.SRem(key, valOrignal).Result()
		if err != nil {
			return err
		}
	case "zset":
		if fieldOrignal == field && val == valOrignal {
			return nil
		}
		s, _ := strconv.ParseFloat(field, 64)
		_, err := r.ZAdd(key, redis.Z{
			Score:  s,
			Member: val,
		}).Result()
		if err != nil {
			return err
		}

		if (val != valOrignal && fieldOrignal == field) || (val != valOrignal && fieldOrignal != field) {
			_, err = r.ZRem(key, valOrignal).Result()
			if err != nil {
				return err
			}
		}
	case "hash":
		if fieldOrignal == field && val == valOrignal {
			return nil
		}
		_, err := r.HSet(key, field, val).Result()
		if err != nil {
			return err
		}
		if fieldOrignal != field {
			_, err = r.HDel(key, fieldOrignal).Result()
			if err != nil {
				return err
			}
		}
	case "list":
		_, err := r.LSet(key, index, val).Result()
		if err != nil {
			return err
		}
	}
	return nil
}

func AddKey(val_1, val_2, key, t string, r *redis.Client) error {
	switch t {
	case "string":
		_, err := r.Set(key, val_2, -1).Result()
		if err != nil {
			return err
		}
	case "bit":
		offset, _ := strconv.Atoi(val_1)
		val, _ := strconv.Atoi(val_2)
		_, err := r.SetBit(key, int64(offset), val).Result()
		if err != nil {
			return err
		}
	case "set":
		_, err := r.SAdd(key, val_2).Result()
		if err != nil {
			return err
		}
	case "zset":
		s, _ := strconv.ParseFloat(val_1, 64)
		_, err := r.ZAdd(key, redis.Z{
			Score:  s,
			Member: val_2,
		}).Result()
		if err != nil {
			return err
		}
	case "hash":
		_, err := r.HSet(key, val_1, val_2).Result()
		if err != nil {
			return err
		}
	case "list":
		_, err := r.LPush(key, val_2).Result()
		if err != nil {
			return err
		}
	}
	return nil
}

func Rename(keyOrignal, key string, r *redis.Client) error {
	_, err := r.Rename(keyOrignal, key).Result()
	if err != nil {
		return err
	}
	return nil
}

func Expire(key string, ttl int, r *redis.Client) error {
	var err error
	if ttl > 0 {
		_, err = r.Expire(key, time.Duration(1000000000*ttl)).Result()
	} else {
		_, err = r.Persist(key).Result()
	}
	if err != nil {
		return err
	}
	return nil
}

func InfoCli(r *redis.Client) (interface{}, error) {
	rst, err := r.Info().Result()
	if err != nil {
		return nil, err
	}
	return rst, err
}

func Del(key string, r *redis.Client) error {
	_, err := r.Del(key).Result()
	if err != nil {
		return err
	}
	return nil
}

func DelCli(id int) error {
	var r []redisServer
	for i := 0; i < len(globalConfigs.Servers); i++ {
		if id != globalConfigs.Servers[i].ID {
			r = append(r, globalConfigs.Servers[i])
		}
	}

	globalConfigs.Servers = r
	err := saveConf()
	if err != nil {
		return err
	}
	return nil
}

func DelLine(key, val, field, t string, r *redis.Client) error {
	switch t {
	case "set":
		_, err := r.SRem(key, val).Result()
		if err != nil {
			return err
		}
	case "zset":
		_, err := r.ZRem(key, val).Result()
		if err != nil {
			return err
		}
	case "hash":
		_, err := r.HDel(key, field).Result()
		if err != nil {
			return err
		}
	case "list":
		_, err := r.LRem(key, 1, val).Result()
		if err != nil {
			return err
		}
	}
	return nil
}

func stringHandle(val []byte) string {
	result := ""
	for _, n := range val {
		result += single(int(n))
	}
	return result
}

func single(n int) string {
	result := ""
	if n == 0 {
		return "00000000"
	}

	for ; n > 0; n /= 2 {
		lsb := n % 2
		result = strconv.Itoa(int(lsb)) + result
	}
	l := ""
	if len(result) < 8 {
		for i := 0; i < 8-len(result); i++ {
			l += "0"
		}
		result = l + result
	}
	return result
}

func keyTypeTtl(c *redis.Client, key string) (string, int, error) {
	s, err := c.Do("TYPE", key).String()
	if err != nil {
		return "", 0, err
	}

	expire, err := c.Do("TTL", key).Int()

	if err != nil {
		return "", 0, err
	}

	return s, expire, err
}

func formatSetAndList(datas []string) []map[string]interface{} {
	tmp := make([]map[string]interface{}, 0, 100)
	for i := 0; i < len(datas); i++ {
		row := make(map[string]interface{})
		row["val"] = datas[i]
		row["index"] = i
		tmp = append(tmp, row)
	}
	return tmp
}

func formatHash(datas []string) []map[string]string {
	tmp := make([]map[string]string, 0, 100)
	for i := 0; i < len(datas); i = i + 2 {
		row := make(map[string]string)
		row["field"] = datas[i]
		row["val"] = datas[i+1]
		tmp = append(tmp, row)
	}
	return tmp
}

func formatZset(datas []string) []map[string]interface{} {
	tmp := make([]map[string]interface{}, 0, 100)
	for i := 0; i < len(datas); i = i + 2 {
		row := make(map[string]interface{})
		row["val"] = datas[i]
		row["score"], _ = strconv.ParseFloat(datas[i+1], 10)
		tmp = append(tmp, row)
	}
	return tmp
}
