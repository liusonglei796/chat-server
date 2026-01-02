package dao

import (
	dao "kama_chat_server/internal/dao/mysql"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/util/random"
	"strconv"
	"testing"
)

func TestCreate(t *testing.T) {
	dao.Init()
	userInfo := &model.UserInfo{
		Uuid:      "U" + strconv.Itoa(random.GetRandomInt(11)),
		Nickname:  "apylee",
		Telephone: "180323532112",
		Email:     "1212312312@qq.com",
		Password:  "123456",
		IsAdmin:   1,
	}
	err := dao.GormDB.Create(userInfo).Error
	if err != nil {
		t.Fatal(err)
	}
}
