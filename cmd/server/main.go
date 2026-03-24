package main

import (
	"fmt"
	"path/filepath"

	"go-llm-demo/configs"
	"go-llm-demo/internal/server/infra/provider"
	"go-llm-demo/internal/server/infra/repository"
	"go-llm-demo/internal/server/infra/tools"
	"go-llm-demo/internal/server/service"
)

func main() {
	workspaceRoot, err := tools.ResolveWorkspaceRoot("")
	if err != nil {
		fmt.Printf("解析工作区失败：%v\n", err)
		return
	}
	if err := tools.SetWorkspaceRoot(workspaceRoot); err != nil {
		fmt.Printf("设置工作区失败：%v\n", err)
		return
	}
	if err := initializeSecurity(filepath.Join(workspaceRoot, "configs", "security")); err != nil {
		fmt.Printf("初始化安全策略失败：%v\n", err)
		return
	}

	if err := configs.LoadAppConfig("config.yaml"); err != nil {
		fmt.Printf("加载配置失败：%v\n", err)
		return
	}

	cfg := configs.GlobalAppConfig
	memoryRepo := repository.NewFileMemoryStore(cfg.Memory.StoragePath, cfg.Memory.MaxItems)
	sessionRepo := repository.NewSessionMemoryStore(cfg.Memory.MaxItems)
	workingRepo := repository.NewWorkingMemoryStore()
	memorySvc := service.NewMemoryService(
		memoryRepo,
		sessionRepo,
		cfg.Memory.TopK,
		cfg.Memory.MinMatchScore,
		cfg.Memory.MaxPromptChars,
		cfg.Memory.StoragePath,
		cfg.Memory.PersistTypes,
	)
	workingSvc := service.NewWorkingMemoryService(workingRepo, cfg.History.ShortTermTurns, tools.GetWorkspaceRoot())

	roleRepo := repository.NewFileRoleStore("./data/roles.json")
	roleSvc := service.NewRoleService(roleRepo, cfg.Persona.FilePath)

	chatProvider, err := provider.NewChatProvider(cfg.AI.Model)
	if err != nil {
		fmt.Printf("初始化 ChatProvider 失败：%v\n", err)
		return
	}

	chatGateway := service.NewChatService(memorySvc, workingSvc, roleSvc, chatProvider)
	fmt.Printf("Server initialized with services: %+v\n", chatGateway)
	fmt.Println("Note: This is a placeholder. Actual server implementation goes here.")
}

func initializeSecurity(configDir string) error {
	securityRepo := repository.NewSecurityConfigRepository()
	securitySvc := service.NewSecurityService(securityRepo)
	if err := securitySvc.Initialize(configDir); err != nil {
		return err
	}
	tools.SetSecurityChecker(securitySvc)
	return nil
}
