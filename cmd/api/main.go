package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"go-be/config"
	"go-be/internal/authz"
	"go-be/internal/handler"
	"go-be/internal/mailer"
	"go-be/internal/middleware"
	"go-be/internal/oauth"
	"go-be/internal/repository"
	"go-be/internal/service"
)

func main() {
	// Load environment
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found")
	}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwtSecret == "" || jwtSecret == "change-me" || len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be set to a strong value (min 32 chars, not the example default)")
	}

	// Connect database
	db, err := config.ConnectDatabase()
	if err != nil {
		log.Fatal("Database connection failed:", err)
	}

	defer db.Close()

	log.Println("PostgreSQL connected successfully!")
	if service.RequireEmailVerification() {
		log.Println("Email verification before login is required")
	} else {
		log.Println("Email verification before login is DISABLED (REQUIRE_EMAIL_VERIFICATION)")
	}

	// Initialize Gin
	router := gin.Default()
	router.Use(middleware.CORS())
	router.Use(middleware.LocaleMiddleware())
	router.Use(middleware.RateLimit(middleware.GlobalRateLimitConfig()))

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	loginAttemptRepo := repository.NewLoginAttemptRepository(db)
	passwordResetRepo := repository.NewPasswordResetRepository(db)
	emailVerificationRepo := repository.NewEmailVerificationRepository(db)
	exportJobRepo := repository.NewExportJobRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	goalRepo := repository.NewGoalRepository(db)
	habitRepo := repository.NewHabitRepository(db)
	plannerRepo := repository.NewPlannerRepository(db)
	calendarRepo := repository.NewCalendarRepository(db)
	noteRepo := repository.NewNoteRepository(db)
	financeRepo := repository.NewFinanceRepository(db)
	reminderRepo := repository.NewReminderRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	billingRepo := repository.NewBillingRepository(db)
	entitlementService := service.NewEntitlementService(billingRepo)

	rbacStore := authz.NewStoreWithDefaults()
	authz.SetStore(rbacStore)
	roleService := service.NewRoleService(roleRepo, rbacStore)
	if err := roleService.ReloadStore(context.Background()); err != nil {
		log.Printf("Warning: failed to load RBAC from database, using defaults: %v", err)
	}

	// Initialize services
	authService := service.NewAuthService(
		userRepo,
		refreshTokenRepo,
		deviceRepo,
		loginAttemptRepo,
		passwordResetRepo,
		emailVerificationRepo,
		mailer.NewFromEnv(),
		os.Getenv("FRONTEND_URL"),
		oauth.NewDiscordClient(oauth.DiscordConfigFromEnv()),
		billingRepo,
	)
	notificationService := service.NewNotificationService(notificationRepo)
	goalService := service.NewGoalService(goalRepo, entitlementService)
	habitService := service.NewHabitService(habitRepo, entitlementService)
	plannerService := service.NewPlannerService(plannerRepo, entitlementService)
	calendarService := service.NewCalendarService(calendarRepo, entitlementService)
	noteService := service.NewNoteService(noteRepo, entitlementService)
	financeService := service.NewFinanceService(financeRepo, entitlementService)
	reminderService := service.NewReminderService(reminderRepo, notificationService, entitlementService)
	analyticsService := service.NewAnalyticsService(goalRepo, habitRepo, plannerRepo)
	exportService := service.NewExportService(exportJobRepo, userRepo, notificationService, os.Getenv("EXPORT_DIR"))
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	exportService.Start(workerCtx, 2)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService)
	notificationHandler := handler.NewNotificationHandler(notificationService)
	goalHandler := handler.NewGoalHandler(goalService)
	habitHandler := handler.NewHabitHandler(habitService)
	plannerHandler := handler.NewPlannerHandler(plannerService)
	calendarHandler := handler.NewCalendarHandler(calendarService)
	noteHandler := handler.NewNoteHandler(noteService)
	financeHandler := handler.NewFinanceHandler(financeService)
	reminderHandler := handler.NewReminderHandler(reminderService)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService)
	adminHandler := handler.NewAdminHandler(authService, userRepo, exportService, db)
	roleHandler := handler.NewRoleHandler(roleService)
	healthHandler := handler.NewHealthHandler(db)
	billingHandler := handler.NewBillingHandler(entitlementService)

	authRateLimit := middleware.RateLimit(middleware.AuthRateLimitConfig())
	authMiddleware := middleware.AuthMiddleware(jwtSecret)

	// Auth routes
	auth := router.Group("/api/auth")
	{
		auth.POST("/register", authRateLimit, authHandler.Register)
		auth.POST("/login", authRateLimit, authHandler.Login)
		auth.POST("/discord", authRateLimit, authHandler.DiscordLogin)
		auth.POST("/refresh", authRateLimit, authHandler.Refresh)
		auth.POST("/forgot-password", authRateLimit, authHandler.ForgotPassword)
		auth.POST("/reset-password", authRateLimit, authHandler.ResetPassword)
		auth.POST("/verify-email", authRateLimit, authHandler.VerifyEmail)
		auth.POST("/resend-verification", authRateLimit, authHandler.ResendVerification)
		auth.POST("/logout", authMiddleware, authHandler.Logout)
	}

	// Protected routes
	protected := router.Group("/api/users")

	protected.Use(authMiddleware)
	{
		protected.GET("/profile", authHandler.GetProfile)
		protected.PATCH("/profile", authHandler.UpdateProfile)
		protected.POST("/change-password", authHandler.ChangePassword)
		protected.GET("/devices", authHandler.ListDevices)
		protected.DELETE("/devices/:id", authHandler.RevokeDevice)
		protected.POST("/sessions/revoke-all", authHandler.LogoutAllSessions)
		protected.GET("/notifications", notificationHandler.List)
		protected.GET("/notifications/unread-count", notificationHandler.UnreadCount)
		protected.POST("/notifications/read-all", notificationHandler.MarkAllRead)
		protected.POST("/notifications/:id/read", notificationHandler.MarkRead)
	}

	// Billing — plans are public; subscription actions require auth
	billing := router.Group("/api/billing")
	billing.GET("/plans", billingHandler.ListPlans)
	billingAuth := billing.Group("")
	billingAuth.Use(authMiddleware)
	{
		billingAuth.GET("/subscription", billingHandler.GetSubscription)
		billingAuth.POST("/upgrade", billingHandler.Upgrade)
		billingAuth.POST("/downgrade", billingHandler.Downgrade)
	}

	// LifeOS — Goal Management (owner-scoped)
	lifeos := router.Group("/api/lifeos")
	lifeos.Use(authMiddleware)
	{
		lifeos.GET("/dashboard", goalHandler.Dashboard)
		lifeos.GET("/dashboard/export.pdf", goalHandler.ExportDashboardPDF)
		lifeos.GET("/goals", goalHandler.List)
		lifeos.GET("/goals/stats", goalHandler.Stats)
		lifeos.POST("/goals", goalHandler.Create)
		lifeos.GET("/goals/:id", goalHandler.Get)
		lifeos.PATCH("/goals/:id", goalHandler.Update)
		lifeos.DELETE("/goals/:id", goalHandler.Delete)
		lifeos.POST("/goals/:id/milestones", goalHandler.AddMilestone)
		lifeos.PATCH("/goals/:id/milestones/:mid", goalHandler.UpdateMilestone)
		lifeos.DELETE("/goals/:id/milestones/:mid", goalHandler.DeleteMilestone)

		lifeos.GET("/habits", habitHandler.List)
		lifeos.GET("/habits/stats", habitHandler.Stats)
		lifeos.GET("/habits/dashboard", habitHandler.Dashboard)
		lifeos.GET("/habits/dashboard/export.pdf", habitHandler.ExportDashboardPDF)
		lifeos.POST("/habits", habitHandler.Create)
		lifeos.GET("/habits/:id", habitHandler.Get)
		lifeos.PATCH("/habits/:id", habitHandler.Update)
		lifeos.DELETE("/habits/:id", habitHandler.Delete)
		lifeos.POST("/habits/:id/checkin", habitHandler.CheckIn)
		lifeos.GET("/habits/:id/logs", habitHandler.ListLogs)

		lifeos.GET("/planner", plannerHandler.List)
		lifeos.GET("/planner/stats", plannerHandler.Stats)
		lifeos.POST("/planner", plannerHandler.Create)
		lifeos.GET("/planner/:id", plannerHandler.Get)
		lifeos.PATCH("/planner/:id", plannerHandler.Update)
		lifeos.POST("/planner/:id/toggle", plannerHandler.Toggle)
		lifeos.DELETE("/planner/:id", plannerHandler.Delete)

		lifeos.GET("/calendar", calendarHandler.List)
		lifeos.GET("/calendar/stats", calendarHandler.Stats)
		lifeos.POST("/calendar", calendarHandler.Create)
		lifeos.GET("/calendar/:id", calendarHandler.Get)
		lifeos.PATCH("/calendar/:id", calendarHandler.Update)
		lifeos.DELETE("/calendar/:id", calendarHandler.Delete)

		lifeos.GET("/notes", noteHandler.List)
		lifeos.GET("/notes/stats", noteHandler.Stats)
		lifeos.POST("/notes", noteHandler.Create)
		lifeos.GET("/notes/:id", noteHandler.Get)
		lifeos.PATCH("/notes/:id", noteHandler.Update)
		lifeos.DELETE("/notes/:id", noteHandler.Delete)

		lifeos.GET("/finance", financeHandler.List)
		lifeos.GET("/finance/stats", financeHandler.Stats)
		lifeos.POST("/finance", financeHandler.Create)
		lifeos.GET("/finance/:id", financeHandler.Get)
		lifeos.PATCH("/finance/:id", financeHandler.Update)
		lifeos.DELETE("/finance/:id", financeHandler.Delete)

		lifeos.GET("/reminders", reminderHandler.List)
		lifeos.POST("/reminders", reminderHandler.Create)
		lifeos.POST("/reminders/check", reminderHandler.Check)
		lifeos.GET("/reminders/:id", reminderHandler.Get)
		lifeos.PATCH("/reminders/:id", reminderHandler.Update)
		lifeos.POST("/reminders/:id/done", reminderHandler.MarkDone)
		lifeos.DELETE("/reminders/:id", reminderHandler.Delete)

		lifeos.GET("/analytics", analyticsHandler.Get)
	}

	// Admin routes — gated by Role → Permission (see internal/authz)
	admin := router.Group("/api/admin")
	admin.Use(authMiddleware, middleware.RequireAdminArea())
	{
		admin.GET("/users", middleware.RequirePermission(authz.PermUsersRead), adminHandler.ListUsers)
		admin.POST("/users/export", middleware.RequirePermission(authz.PermUsersExport), adminHandler.ExportUsers)
		admin.GET("/users/export/jobs/:id", middleware.RequirePermission(authz.PermUsersExport), adminHandler.GetExportJob)
		admin.GET("/users/export/jobs/:id/download", middleware.RequirePermission(authz.PermUsersExport), adminHandler.DownloadExportJob)
		admin.POST("/users", middleware.RequirePermission(authz.PermUsersCreate), adminHandler.CreateUser)
		admin.PATCH("/users/:id", middleware.RequirePermission(authz.PermUsersUpdate), adminHandler.UpdateUser)
		admin.PATCH("/users/:id/subscription", middleware.RequirePermission(authz.PermUsersUpdate), billingHandler.AdminUpdateSubscription)
		admin.GET("/stats", middleware.RequirePermission(authz.PermAdminStats), adminHandler.Stats)
		admin.DELETE("/users/:id", middleware.RequirePermission(authz.PermUsersDelete), adminHandler.DeleteUser)
		admin.POST("/users/:id/restore", middleware.RequirePermission(authz.PermUsersRestore), adminHandler.RestoreUser)
		admin.POST("/users/:id/kick", middleware.RequirePermission(authz.PermUsersKick), adminHandler.KickUser)
		admin.POST("/users/:id/reset-password", middleware.RequirePermission(authz.PermUsersResetPassword), adminHandler.SetPassword)
		admin.POST("/users/:id/send-reset-password", middleware.RequirePermission(authz.PermUsersResetPassword), adminHandler.SendResetPassword)
		admin.POST("/users/:id/verify-email", middleware.RequirePermission(authz.PermUsersVerifyEmail), adminHandler.VerifyEmail)

		admin.GET("/permissions", middleware.RequirePermission(authz.PermRolesRead), roleHandler.ListPermissions)
		admin.GET("/roles", middleware.RequireAnyPermission(authz.PermRolesRead, authz.PermUsersRead, authz.PermUsersCreate, authz.PermUsersUpdate), roleHandler.ListRoles)
		admin.GET("/roles/:id", middleware.RequirePermission(authz.PermRolesRead), roleHandler.GetRole)
		admin.POST("/roles", middleware.RequirePermission(authz.PermRolesCreate), roleHandler.CreateRole)
		admin.PATCH("/roles/:id", middleware.RequirePermission(authz.PermRolesUpdate), roleHandler.UpdateRole)
		admin.DELETE("/roles/:id", middleware.RequirePermission(authz.PermRolesDelete), roleHandler.DeleteRole)
	}

	// Health check
	health := router.Group("/api/health")
	{
		health.GET("/", healthHandler.HealthCheck)
		health.GET("/db", healthHandler.DatabaseHealthCheck)
	}

	// Start server
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		log.Println("Server running on port:", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed:", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")
	workerCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
}
