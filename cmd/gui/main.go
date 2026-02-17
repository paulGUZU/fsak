package main

import (
	"fmt"
	"log"
	"os"

	fyneApp "fyne.io/fyne/v2/app"

	"github.com/paulGUZU/fsak/cmd/gui/internal/app"
	"github.com/paulGUZU/fsak/cmd/gui/internal/models"
	"github.com/paulGUZU/fsak/cmd/gui/internal/services"
	"github.com/paulGUZU/fsak/cmd/gui/internal/ui"
)

func main() {
	// Check for TUN helper mode
	if len(os.Args) > 1 && os.Args[1] == models.TunHelperArg {
		if err := services.RunTUNHelper(os.Args[2:]); err != nil {
			log.Fatalf("TUN helper failed: %v", err)
		}
		return
	}

	// Initialize application
	if err := run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}

func run() error {
	// Get storage path
	storePath, err := services.DefaultStorePath()
	if err != nil {
		return fmt.Errorf("failed to resolve storage path: %w", err)
	}

	// Initialize services
	profileSvc := services.NewProfileService(storePath)

	// Load profiles
	profiles, selected, err := profileSvc.LoadProfiles()
	if err != nil {
		return fmt.Errorf("failed to load profiles: %w", err)
	}

	// Initialize state
	state := models.NewGUIState()
	state.ReplaceProfiles(profiles, selected)

	// Initialize runner service
	runnerSvc := services.NewRunnerService(state)

	// Create Fyne app
	application := fyneApp.NewWithID(models.AppID)
	application.Settings().SetTheme(app.NewVibrantTheme())

	// Create main window
	mainWindow := ui.NewMainWindow(application, state, profileSvc, runnerSvc)

	// Run
	mainWindow.ShowAndRun()

	return nil
}
