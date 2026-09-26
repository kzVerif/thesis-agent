package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"ws-agent/internal/agent"
	"ws-agent/internal/apppaths"
	"ws-agent/internal/protectedpath"
	"ws-agent/internal/runlock"
	"ws-agent/internal/servicehost"
	"ws-agent/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	serviceFlag := flag.Bool("service", false, "run only when launched by Windows SCM")
	provision := flag.Bool("provision", false, "administrative interactive enrollment using protected machine paths")
	migrate := flag.String("migrate-identity", "", "explicit absolute existing identity path; provisioning only")
	metadata := flag.Bool("service-info", false, "print centralized development Service metadata for admin tooling")
	configure := flag.Bool("configure-service", false, "administrative development Service registration; use provisioning script")
	migrateKey := flag.Bool("migrate-private-key-protection", false, "explicit administrative legacy-to-machine DPAPI migration; Service must be stopped")
	verifyKey := flag.Bool("verify-private-key", false, "verify protected Service private key without changing identity or contacting servers; Service must be stopped")
	repairACL := flag.Bool("repair-runtime-acl", false, "administrative safe runtime ACL repair only; use dev-service.ps1 -Action Repair")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected command arguments")
	}
	if *repairACL {
		if *migrateKey || *verifyKey || *configure || *provision || *serviceFlag || *metadata || *migrate != "" {
			return fmt.Errorf("ACL repair cannot be combined with other modes")
		}
		return repairRuntimeACL()
	}
	if *migrateKey || *verifyKey {
		if (*migrateKey && *verifyKey) || *configure || *provision || *serviceFlag || *metadata || *migrate != "" {
			return fmt.Errorf("private-key maintenance cannot be combined with other modes")
		}
		return maintainPrivateKey(*migrateKey)
	}
	if *configure {
		if *provision || *serviceFlag || *metadata || *migrate != "" {
			return fmt.Errorf("configure-service cannot be combined with other modes")
		}
		return servicehost.Configure()
	}
	if *metadata {
		paths, err := apppaths.Machine()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Name        string         `json:"name"`
			DisplayName string         `json:"display_name"`
			Description string         `json:"description"`
			Executable  string         `json:"executable"`
			Paths       apppaths.Paths `json:"paths"`
		}{servicehost.Name, servicehost.DisplayName, servicehost.Description, filepath.Join(paths.Install, "thesis-agent.exe"), paths})
	}
	inService, err := servicehost.IsService()
	if err != nil {
		return err
	}
	if *serviceFlag && !inService {
		return fmt.Errorf("--service must be launched by Windows Service Control Manager")
	}
	if inService {
		if *provision || *migrate != "" {
			return fmt.Errorf("SCM cannot perform interactive provisioning")
		}
		return servicehost.Run(func(ctx context.Context, ready func()) error {
			return agent.Run(ctx, agent.Options{Service: true}, ready)
		})
	}
	if *provision {
		if err := servicehost.RequireAdministrator(); err != nil {
			return err
		}
	}
	if *migrate != "" && !*provision {
		return fmt.Errorf("--migrate-identity requires --provision")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agent.Run(ctx, agent.Options{Provision: *provision, MigrationSource: *migrate}, nil)
}

func repairRuntimeACL() error {
	if err := servicehost.RequireAdministrator(); err != nil {
		return err
	}
	inService, err := servicehost.IsService()
	if err != nil {
		return err
	}
	if inService {
		return fmt.Errorf("SCM cannot perform administrative ACL repair")
	}
	if err := servicehost.RequireStoppedOwnedService(); err != nil {
		return err
	}
	paths, err := apppaths.Machine()
	if err != nil {
		return err
	}
	unlock, err := protectedpath.EnsureRuntimeSecurity(context.Background(), paths, func(d protectedpath.Diagnostic) {
		fmt.Println(d.String())
		servicehost.ReportStartupDiagnostic(d.String(), d.Action == "fail_closed")
	})
	if err != nil {
		return err
	}
	defer unlock()
	fmt.Println("Runtime ACL verified; identity/config/enrollment contents untouched. Service remains stopped.")
	return nil
}

func maintainPrivateKey(migrate bool) error {
	if err := servicehost.RequireAdministrator(); err != nil {
		return err
	}
	inService, err := servicehost.IsService()
	if err != nil {
		return err
	}
	if inService {
		return fmt.Errorf("SCM cannot perform private-key maintenance")
	}
	if err := servicehost.RequireStoppedForKeyMaintenance(); err != nil {
		return err
	}
	paths, err := apppaths.Machine()
	if err != nil {
		return err
	}
	if migrate {
		result, err := service.MigratePrivateKeyProtection(paths.Identity, paths.Enrollment)
		if err != nil {
			return err
		}
		if result.AlreadyMigrated {
			fmt.Println("machine-protected private key verified; identity unchanged")
		} else {
			fmt.Println("private-key protection migrated and verified; Agent ID and exact key pair preserved")
			fmt.Println("encrypted identity backup retained:", result.BackupPath)
		}
		return nil
	}
	unlock, err := runlock.Acquire(filepath.Join(paths.Root, ".runtime.lock"))
	if err != nil {
		return fmt.Errorf("Service/runtime must be stopped before private-key verification")
	}
	defer unlock()
	key, err := service.LoadPrivateKey(paths.Identity)
	if err != nil {
		return err
	}
	clear(key)
	fmt.Println("machine-protected private key matches stored public key; identity unchanged")
	return nil
}
