package gateway

import (
	"time"

	"github.com/gin-gonic/gin"

	"kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/middleware"
)

// Router 路由管理器
type Router struct {
	handlers *Handlers
	cache    cache.CacheService
}

// NewRouter 创建路由管理器
func NewRouter(handlers *Handlers, cache cache.CacheService) *Router {
	return &Router{
		handlers: handlers,
		cache:    cache,
	}
}

// RegisterRoutes 注册所有路由
func (rt *Router) RegisterRoutes(r *gin.Engine) {
	// ==================== 公开路由 (无需认证) ====================
	public := r.Group("")
	{
		rt.registerAuthRoutes(public)
	}

	// ==================== 私有路由 (需要认证) ====================
	private := r.Group("")
	private.Use(middleware.JWTAuthWithCache(rt.cache))
	{
		rt.registerUserRoutes(private)
		rt.registerFriendRoutes(private)
		rt.registerGroupRoutes(private)
		rt.registerSessionRoutes(private)
		rt.registerMessageRoutes(private)
		rt.registerWebSocketRoutes(private)
		rt.registerAuthPrivateRoutes(private)
	}
}

func (rt *Router) registerAuthRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		loginLimiter := middleware.RateLimit(rt.cache, "rate:login:", middleware.ByClientIP, 100, 5*time.Minute)
		authGroup.POST("/login", loginLimiter, rt.handlers.User.Login)

		registerLimiter := middleware.RateLimit(rt.cache, "rate:register:", middleware.ByClientIP, 100, 5*time.Minute)
		authGroup.POST("/register", registerLimiter, rt.handlers.User.Register)

		authGroup.POST("/refresh", rt.handlers.Auth.RefreshToken)
	}
}

func (rt *Router) registerAuthPrivateRoutes(rg *gin.RouterGroup) {
	authGroup := rg.Group("/auth")
	{
		authGroup.POST("/logout", rt.handlers.User.Logout)
		authGroup.POST("/kick", rt.handlers.User.KickUser)
	}
}

func (rt *Router) registerUserRoutes(rg *gin.RouterGroup) {
	userGroup := rg.Group("/user")
	{
		userGroup.PUT("/info", rt.handlers.User.UpdateUserInfo)
		userGroup.GET("/info", rt.handlers.User.GetUserInfo)
		userGroup.GET("/public-info", rt.handlers.User.GetPublicUserInfo)
	}
}

func (rt *Router) registerFriendRoutes(rg *gin.RouterGroup) {
	friendGroup := rg.Group("/friends")
	{
		friendGroup.GET("", rt.handlers.Friendship.GetFriendList)
		friendGroup.GET("/info", rt.handlers.Friendship.GetFriendInfo)

		friendGroup.DELETE("", rt.handlers.Friendship.DeleteFriend)
		friendGroup.PUT("/block", rt.handlers.Friendship.BlockFriend)
		friendGroup.DELETE("/block", rt.handlers.Friendship.UnblockFriend)
		friendGroup.PUT("/remark", rt.handlers.Friendship.UpdateRemark)

		friendGroup.POST("/apply", rt.handlers.Apply.ApplyFriend)
		friendGroup.GET("/applies", rt.handlers.Apply.GetFriendApplyList)
		friendGroup.POST("/applies/approve", rt.handlers.Apply.PassFriendApply)
		friendGroup.POST("/applies/refuse", rt.handlers.Apply.RefuseFriendApply)
		friendGroup.POST("/applies/block", rt.handlers.Apply.BlackFriendApply)
	}
}

func (rt *Router) registerGroupRoutes(rg *gin.RouterGroup) {
	groupGroup := rg.Group("/groups")
	{
		groupGroup.POST("", rt.handlers.Group.CreateGroup)
		groupGroup.GET("/owned", rt.handlers.Group.LoadMyGroup)
		groupGroup.GET("/joined", rt.handlers.Friendship.LoadMyJoinedGroup)
		groupGroup.GET("/detail", rt.handlers.Group.GetGroupDetail)
		groupGroup.PUT("/info", rt.handlers.Group.UpdateGroupInfo)
		groupGroup.DELETE("", rt.handlers.Group.DismissGroup)
		groupGroup.POST("/leave", rt.handlers.Group.LeaveGroup)

		groupGroup.GET("/members", rt.handlers.Group.GetGroupMemberList)
		groupGroup.DELETE("/members", rt.handlers.Group.RemoveGroupMembers)
		groupGroup.PUT("/members/mute", rt.handlers.Group.MuteMember)

		groupGroup.GET("/add-mode", rt.handlers.Group.CheckGroupAddMode)
		groupGroup.POST("/apply", rt.handlers.Apply.ApplyGroup)
		groupGroup.GET("/applies", rt.handlers.Apply.GetGroupApplyList)
		groupGroup.POST("/applies/approve", rt.handlers.Apply.PassGroupApply)
		groupGroup.POST("/applies/refuse", rt.handlers.Apply.RefuseGroupApply)
		groupGroup.POST("/applies/block", rt.handlers.Apply.BlackGroupApply)
	}
}

func (rt *Router) registerSessionRoutes(rg *gin.RouterGroup) {
	sessionGroup := rg.Group("/sessions")
	{
		sessionGroup.GET("/check", rt.handlers.Session.CheckOpenSessionAllowed)
		sessionGroup.POST("", rt.handlers.Session.OpenSession)
		sessionGroup.GET("/direct", rt.handlers.Session.GetUserSessionList)
		sessionGroup.GET("/group", rt.handlers.Session.GetGroupSessionList)
		sessionGroup.DELETE("", rt.handlers.Session.DeleteSession)
		sessionGroup.PUT("/pin", rt.handlers.Session.PinSession)
	}
}

func (rt *Router) registerMessageRoutes(rg *gin.RouterGroup) {
	messageGroup := rg.Group("/messages")
	{
		messageGroup.GET("", rt.handlers.Message.GetMessageList)
		messageGroup.POST("/recall", rt.handlers.Message.RecallMessage)
	}

	uploadGroup := rg.Group("/upload")
	{
		uploadGroup.POST("/avatar", rt.handlers.Message.UploadAvatar)
		uploadGroup.POST("/file", rt.handlers.Message.UploadFile)
	}
}

func (rt *Router) registerWebSocketRoutes(rg *gin.RouterGroup) {
	wsGroup := rg.Group("/ws")
	{
		wsGroup.GET("", rt.handlers.Ws.WsLoginHandler)
		wsGroup.POST("/logout", rt.handlers.Ws.WsLogoutHandler)
	}
}
