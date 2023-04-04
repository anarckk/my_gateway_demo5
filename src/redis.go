package src

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"

	redis "github.com/redis/go-redis/v9"
)

const ( // redis key
	InviteCodesKey = "InviteCodesKey" // 邀请码，一个邀请码换一个允许访问的用户
	AllowUserKey   = "AllowUserKey"   // 允许访问的用户
)

type RedisController struct {
	RedisClient *redis.Client
}

func (redisController *RedisController) Init(address string) {
	redisController.RedisClient = redis.NewClient(&redis.Options{
		Addr:     address,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
}

// 检查邀请码是否合法
// 如果许可，则会删除掉redis上的邀请码，并返回true
func (redisController *RedisController) CheckInviteCode(ctx context.Context, inviteCode string) (bool, error) {
	isMember, err := redisController.RedisClient.SIsMember(ctx, InviteCodesKey, inviteCode).Result()
	if err != nil {
		return false, err
	}
	if isMember {
		i, err := redisController.RedisClient.SRem(ctx, InviteCodesKey, inviteCode).Result()
		if err != nil {
			return false, err
		}
		if i == 1 {
			return true, nil
		} else {
			log.Println("被删除的邀请码数量不是1个")
			return false, errors.New("error")
		}
	}
	return false, nil
}

// 如果许可，则增加一个用户ID
func (redisController *RedisController) AddUser(ctx context.Context, userId string) error {
	i, err := redisController.RedisClient.SAdd(ctx, AllowUserKey, userId).Result()
	if err != nil {
		return err
	}
	if i == 1 {
		return nil
	} else {
		log.Println("增加的用户不是1个")
		return errors.New("error")
	}
}

// 检查用户ID是否合法
func (redisController *RedisController) CheckUser(ctx context.Context, userId string) (bool, error) {
	isMember, err := redisController.RedisClient.SIsMember(ctx, AllowUserKey, userId).Result()
	if err != nil {
		return false, err
	}
	return isMember, nil
}

// 统计当前许可的当前用户总数量
func (redisController *RedisController) UserSize(ctx context.Context) (int64, error) {
	return redisController.RedisClient.SCard(ctx, AllowUserKey).Result()
}

// 检查用户是否有权访问此服务
// 1-cxx-gogs 根据用户ID规则来看是否有权访问, cxx 是用户，gogs是许可访问的服务
// 0-me-all me是我，all是允许访问全部
func CheckAuthorization(svrName string, userId string) bool {
	services := ExtractUserId(userId)
	for _, svr := range services {
		if svr == "all" {
			return true
		}
		if "svr-"+svr == svrName {
			return true
		}
	}
	return false
}

// 从用户ID中提取它被许可的服务名列表
func ExtractUserId(userId string) []string {
	re := regexp.MustCompile(`^\d-[a-zA-Z]+-([a-zA-Z\d_]+)$`)
	matched := re.FindStringSubmatch(userId)
	if len(matched) >= 2 {
		services := matched[1]
		return strings.Split(services, "_")
	} else {
		log.Printf("No match found in string [%s]\n", userId)
	}
	return []string{}
}
