package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kama_chat_server/internal/dao/mysql/repository"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/handler"
	"kama_chat_server/internal/https_server"
	"kama_chat_server/internal/service"
	chat "kama_chat_server/internal/service/chat"
	"kama_chat_server/pkg/util/jwt"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type stubUserService struct{}

type stubSessionService struct{}

type stubGroupService struct{}

type stubContactService struct{}

type stubMessageService struct{}

type stubAuthService struct{}

func (s stubUserService) Login(req request.LoginRequest) (*respond.LoginRespond, error) {
	return &respond.LoginRespond{}, nil
}
func (s stubUserService) SmsLogin(req request.SmsLoginRequest) (*respond.LoginRespond, error) {
	return &respond.LoginRespond{}, nil
}
func (s stubUserService) SendSmsCode(telephone string) error { return nil }
func (s stubUserService) Register(req request.RegisterRequest) (*respond.RegisterRespond, error) {
	return &respond.RegisterRespond{}, nil
}
func (s stubUserService) UpdateUserInfo(req request.UpdateUserInfoRequest) error { return nil }
func (s stubUserService) GetUserInfoList(ownerId string) ([]respond.GetUserListRespond, error) {
	return []respond.GetUserListRespond{}, nil
}
func (s stubUserService) AbleUsers(uuidList []string) error    { return nil }
func (s stubUserService) DisableUsers(uuidList []string) error { return nil }
func (s stubUserService) DeleteUsers(uuidList []string) error  { return nil }
func (s stubUserService) GetUserInfo(uuid string) (*respond.GetUserInfoRespond, error) {
	return &respond.GetUserInfoRespond{}, nil
}
func (s stubUserService) SetAdmin(uuidList []string, isAdmin int8) error { return nil }

func (s stubSessionService) CreateSession(req request.CreateSessionRequest) (string, error) {
	return "S_TEST", nil
}
func (s stubSessionService) CheckOpenSessionAllowed(sendId, receiveId string) (bool, error) {
	return true, nil
}
func (s stubSessionService) OpenSession(req request.OpenSessionRequest) (string, error) {
	return "S_TEST", nil
}
func (s stubSessionService) GetUserSessionList(ownerId string) ([]respond.UserSessionListRespond, error) {
	return []respond.UserSessionListRespond{}, nil
}
func (s stubSessionService) GetGroupSessionList(ownerId string) ([]respond.GroupSessionListRespond, error) {
	return []respond.GroupSessionListRespond{}, nil
}
func (s stubSessionService) DeleteSession(ownerId, sessionId string) error { return nil }

func (s stubGroupService) CreateGroup(req request.CreateGroupRequest) error { return nil }
func (s stubGroupService) LoadMyGroup(ownerId string) ([]respond.LoadMyGroupRespond, error) {
	return []respond.LoadMyGroupRespond{}, nil
}
func (s stubGroupService) CheckGroupAddMode(groupId string) (int8, error)  { return 0, nil }
func (s stubGroupService) EnterGroupDirectly(groupId, userId string) error { return nil }
func (s stubGroupService) LeaveGroup(userId, groupId string) error         { return nil }
func (s stubGroupService) DismissGroup(ownerId, groupId string) error      { return nil }
func (s stubGroupService) GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error) {
	return &respond.GetGroupInfoRespond{}, nil
}
func (s stubGroupService) GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error) {
	return &respond.GetGroupListWrapper{}, nil
}
func (s stubGroupService) DeleteGroups(uuidList []string) error { return nil }
func (s stubGroupService) SetGroupsStatus(uuidList []string, status int8) error {
	return nil
}
func (s stubGroupService) UpdateGroupInfo(req request.UpdateGroupInfoRequest) error { return nil }
func (s stubGroupService) GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error) {
	return []respond.GetGroupMemberListRespond{}, nil
}
func (s stubGroupService) RemoveGroupMembers(req request.RemoveGroupMembersRequest) error { return nil }

func (s stubContactService) GetUserList(userId string) ([]respond.MyUserListRespond, error) {
	return []respond.MyUserListRespond{}, nil
}
func (s stubContactService) GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	return []respond.LoadMyJoinedGroupRespond{}, nil
}
func (s stubContactService) GetFriendInfo(friendId string) (respond.GetFriendInfoRespond, error) {
	return respond.GetFriendInfoRespond{}, nil
}
func (s stubContactService) GetGroupDetail(groupId string) (respond.GetGroupDetailRespond, error) {
	return respond.GetGroupDetailRespond{}, nil
}
func (s stubContactService) DeleteContact(userId, contactId string) error     { return nil }
func (s stubContactService) ApplyFriend(req request.ApplyFriendRequest) error { return nil }
func (s stubContactService) GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error) {
	return []respond.NewContactListRespond{}, nil
}
func (s stubContactService) PassFriendApply(userId, applicantId string) error { return nil }
func (s stubContactService) RefuseFriendApply(userId, applicantId string) error {
	return nil
}
func (s stubContactService) BlackFriendApply(userId, applicantId string) error { return nil }
func (s stubContactService) ApplyGroup(req request.ApplyGroupRequest) error    { return nil }
func (s stubContactService) GetGroupApplyList(groupId string) ([]respond.AddGroupListRespond, error) {
	return []respond.AddGroupListRespond{}, nil
}
func (s stubContactService) PassGroupApply(groupId, applicantId string) error { return nil }
func (s stubContactService) RefuseGroupApply(groupId, applicantId string) error {
	return nil
}
func (s stubContactService) BlackGroupApply(groupId, applicantId string) error { return nil }
func (s stubContactService) BlackContact(userId, contactId string) error       { return nil }
func (s stubContactService) CancelBlackContact(userId, contactId string) error { return nil }

func (s stubMessageService) GetMessageList(userOneId, userTwoId string) ([]respond.GetMessageListRespond, error) {
	return []respond.GetMessageListRespond{}, nil
}
func (s stubMessageService) GetGroupMessageList(groupId string) ([]respond.GetGroupMessageListRespond, error) {
	return []respond.GetGroupMessageListRespond{}, nil
}
func (s stubMessageService) UploadAvatar(c *gin.Context) (string, error) { return "avatar.png", nil }
func (s stubMessageService) UploadFile(c *gin.Context) ([]string, error) {
	return []string{"file.bin"}, nil
}

func (s stubAuthService) ValidateTokenID(userID, tokenID string) (bool, error) { return true, nil }

type stubBroker struct {
	clients sync.Map
}

func (b *stubBroker) Publish(ctx context.Context, msg []byte) error { return nil }
func (b *stubBroker) RegisterClient(client *chat.UserConn)          { b.clients.Store(client.Uuid, client) }
func (b *stubBroker) UnregisterClient(client *chat.UserConn)        { b.clients.Delete(client.Uuid) }
func (b *stubBroker) GetClient(userId string) *chat.UserConn {
	if v, ok := b.clients.Load(userId); ok {
		return v.(*chat.UserConn)
	}
	return nil
}
func (b *stubBroker) Start()                                       {}
func (b *stubBroker) Close()                                       {}
func (b *stubBroker) GetMessageRepo() repository.MessageRepository { return nil }

func mustJSON(t *testing.T, v any) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func doReq(t *testing.T, client *http.Client, method, url string, body io.Reader, authHeader string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

func requireNot5xxOr404(t *testing.T, path string, resp *http.Response) {
	t.Helper()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= 500 {
		t.Fatalf("%s status=%d", path, resp.StatusCode)
	}
}

func TestAllHTTPAndWebSocketEndpoints_Smoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt.Init("test-secret", 15, 168)

	broker := &stubBroker{}
	svcs := &service.Services{
		User:    stubUserService{},
		Session: stubSessionService{},
		Group:   stubGroupService{},
		Contact: stubContactService{},
		Message: stubMessageService{},
		Auth:    stubAuthService{},
	}

	engine := https_server.Init(handler.NewHandlers(svcs, broker))
	server := httptest.NewServer(engine)
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	accessToken, err := jwt.GenerateAccessToken("U_TEST")
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	authHeader := "Bearer " + accessToken

	refreshToken, _, err := jwt.GenerateRefreshToken("U_TEST")
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}

	// ===== 公共接口（无需鉴权） =====
	resp := doReq(t, client, http.MethodPost, server.URL+"/login", mustJSON(t, map[string]any{
		"telephone": "13000000000",
		"password":  "123456",
	}), "")
	requireNot5xxOr404(t, "/login", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/register", mustJSON(t, map[string]any{
		"telephone": "13000000001",
		"password":  "123456",
		"nickname":  "n",
		"sms_code":  "123456",
	}), "")
	requireNot5xxOr404(t, "/register", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/user/smsLogin", mustJSON(t, map[string]any{
		"telephone": "13000000002",
		"sms_code":  "123456",
	}), "")
	requireNot5xxOr404(t, "/user/smsLogin", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/user/sendSmsCode", mustJSON(t, map[string]any{
		"telephone": "13000000003",
	}), "")
	requireNot5xxOr404(t, "/user/sendSmsCode", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/auth/refresh", mustJSON(t, map[string]any{
		"refresh_token": refreshToken,
	}), "")
	requireNot5xxOr404(t, "/auth/refresh", resp)
	_ = resp.Body.Close()

	// ===== 私有接口（需要鉴权） =====
	resp = doReq(t, client, http.MethodGet, server.URL+"/admin/user/list?owner_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/admin/user/list", resp)
	_ = resp.Body.Close()

	for _, path := range []string{"/admin/user/setAdmin", "/admin/user/able", "/admin/user/disable", "/admin/user/delete"} {
		resp = doReq(t, client, http.MethodPost, server.URL+path, mustJSON(t, map[string]any{
			"uuid_list": []string{"U_1"},
			"is_admin":  1,
		}), authHeader)
		requireNot5xxOr404(t, path, resp)
		_ = resp.Body.Close()
	}

	resp = doReq(t, client, http.MethodGet, server.URL+"/admin/group/list?page=1&page_size=10", nil, authHeader)
	requireNot5xxOr404(t, "/admin/group/list", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/admin/group/delete", mustJSON(t, map[string]any{
		"uuidList": []string{"G_1"},
	}), authHeader)
	requireNot5xxOr404(t, "/admin/group/delete", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/admin/group/setStatus", mustJSON(t, map[string]any{
		"uuid_list": []string{"G_1"},
		"status":    1,
	}), authHeader)
	requireNot5xxOr404(t, "/admin/group/setStatus", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/user/updateUserInfo", mustJSON(t, map[string]any{
		"uuid": "U_TEST",
	}), authHeader)
	requireNot5xxOr404(t, "/user/updateUserInfo", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/user/getUserInfo?uuid=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/user/getUserInfo", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/friend/list?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/friend/list", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/friend/info?friend_id=U_2", nil, authHeader)
	requireNot5xxOr404(t, "/friend/info", resp)
	_ = resp.Body.Close()

	for _, path := range []string{"/friend/delete", "/friend/black", "/friend/cancelBlack"} {
		resp = doReq(t, client, http.MethodPost, server.URL+path, mustJSON(t, map[string]any{
			"user_id":    "U_TEST",
			"contact_id": "U_2",
		}), authHeader)
		requireNot5xxOr404(t, path, resp)
		_ = resp.Body.Close()
	}

	resp = doReq(t, client, http.MethodPost, server.URL+"/friend/apply", mustJSON(t, map[string]any{
		"user_id":   "U_TEST",
		"friend_id": "U_2",
		"message":   "hi",
	}), authHeader)
	requireNot5xxOr404(t, "/friend/apply", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/friend/applyList?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/friend/applyList", resp)
	_ = resp.Body.Close()

	for _, path := range []string{"/friend/passApply", "/friend/refuseApply", "/friend/blackApply"} {
		resp = doReq(t, client, http.MethodPost, server.URL+path, mustJSON(t, map[string]any{
			"user_id":      "U_TEST",
			"applicant_id": "U_3",
		}), authHeader)
		requireNot5xxOr404(t, path, resp)
		_ = resp.Body.Close()
	}

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/createGroup", mustJSON(t, map[string]any{
		"owner_id": "U_TEST",
		"name":     "G",
	}), authHeader)
	requireNot5xxOr404(t, "/group/createGroup", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/loadMyGroup?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/group/loadMyGroup", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/loadMyJoinedGroup?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/group/loadMyJoinedGroup", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/getGroupInfo?group_id=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/group/getGroupInfo", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/getGroupDetail?group_id=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/group/getGroupDetail", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/updateGroupInfo", mustJSON(t, map[string]any{
		"owner_id": "U_TEST",
		"uuid":     "G_1",
		"name":     "G2",
	}), authHeader)
	requireNot5xxOr404(t, "/group/updateGroupInfo", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/dismissGroup", mustJSON(t, map[string]any{
		"owner_id": "U_TEST",
		"group_id": "G_1",
	}), authHeader)
	requireNot5xxOr404(t, "/group/dismissGroup", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/leaveGroup", mustJSON(t, map[string]any{
		"user_id":  "U_TEST",
		"group_id": "G_1",
	}), authHeader)
	requireNot5xxOr404(t, "/group/leaveGroup", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/getGroupMemberList?group_id=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/group/getGroupMemberList", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/removeGroupMembers", mustJSON(t, map[string]any{
		"group_id":  "G_1",
		"owner_id":  "U_TEST",
		"uuid_list": []string{"U_2"},
	}), authHeader)
	requireNot5xxOr404(t, "/group/removeGroupMembers", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/checkGroupAddMode?group_id=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/group/checkGroupAddMode", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/enterGroupDirectly", mustJSON(t, map[string]any{
		"user_id":  "U_TEST",
		"group_id": "G_1",
	}), authHeader)
	requireNot5xxOr404(t, "/group/enterGroupDirectly", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/group/apply", mustJSON(t, map[string]any{
		"user_id":  "U_TEST",
		"group_id": "G_1",
		"message":  "join",
	}), authHeader)
	requireNot5xxOr404(t, "/group/apply", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/group/applyList?group_id=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/group/applyList", resp)
	_ = resp.Body.Close()

	for _, path := range []string{"/group/passApply", "/group/refuseApply", "/group/blackApply"} {
		resp = doReq(t, client, http.MethodPost, server.URL+path, mustJSON(t, map[string]any{
			"group_id":     "G_1",
			"applicant_id": "U_3",
		}), authHeader)
		requireNot5xxOr404(t, path, resp)
		_ = resp.Body.Close()
	}

	resp = doReq(t, client, http.MethodGet, server.URL+"/session/checkOpenSessionAllowed?send_id=U_TEST&receive_id=U_2", nil, authHeader)
	requireNot5xxOr404(t, "/session/checkOpenSessionAllowed", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/session/openSession", mustJSON(t, map[string]any{
		"send_id":    "U_TEST",
		"receive_id": "U_2",
	}), authHeader)
	requireNot5xxOr404(t, "/session/openSession", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/session/getUserSessionList?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/session/getUserSessionList", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/session/getGroupSessionList?user_id=U_TEST", nil, authHeader)
	requireNot5xxOr404(t, "/session/getGroupSessionList", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/session/deleteSession", mustJSON(t, map[string]any{
		"user_id":    "U_TEST",
		"session_id": "S_TEST",
	}), authHeader)
	requireNot5xxOr404(t, "/session/deleteSession", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/message/getMessageList?userOneId=U_TEST&userTwoId=U_2", nil, authHeader)
	requireNot5xxOr404(t, "/message/getMessageList", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodGet, server.URL+"/message/getGroupMessageList?groupId=G_1", nil, authHeader)
	requireNot5xxOr404(t, "/message/getGroupMessageList", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/message/uploadAvatar", mustJSON(t, map[string]any{}), authHeader)
	requireNot5xxOr404(t, "/message/uploadAvatar", resp)
	_ = resp.Body.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/message/uploadFile", mustJSON(t, map[string]any{}), authHeader)
	requireNot5xxOr404(t, "/message/uploadFile", resp)
	_ = resp.Body.Close()

	// ===== WebSocket 接口（需要鉴权） =====
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/wss?client_id=U_TEST"
	headers := http.Header{}
	headers.Set("Authorization", authHeader)
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	_ = wsConn.Close()

	resp = doReq(t, client, http.MethodPost, server.URL+"/user/wsLogout", mustJSON(t, map[string]any{
		"owner_id": "U_TEST",
	}), authHeader)
	requireNot5xxOr404(t, "/user/wsLogout", resp)
	_ = resp.Body.Close()
}
