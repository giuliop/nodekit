package node

import (
	"context"
	"fmt"
	"github.com/algorandfoundation/algorun-tui/api"
	cmdutils "github.com/algorandfoundation/algorun-tui/cmd/utils"
	"github.com/algorandfoundation/algorun-tui/internal/algod"
	"github.com/algorandfoundation/algorun-tui/internal/algod/utils"
	"github.com/algorandfoundation/algorun-tui/internal/system"
	"github.com/algorandfoundation/algorun-tui/ui"
	"github.com/algorandfoundation/algorun-tui/ui/app"
	"github.com/algorandfoundation/algorun-tui/ui/bootstrap"
	"github.com/algorandfoundation/algorun-tui/ui/style"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"time"
)

var in = `# Welcome!

This is the beginning of your adventure into running the an Algorand node!

Morbi mauris quam, ornare ac commodo et, posuere id sem. Nulla id condimentum mauris. In vehicula sit amet libero vitae interdum. Nullam ac massa in erat volutpat sodales. Integer imperdiet enim cursus, ullamcorper tortor vel, imperdiet diam. Maecenas viverra ex iaculis, vehicula ligula quis, cursus lorem. Mauris nec nunc feugiat tortor sollicitudin porta ac quis turpis. Nam auctor hendrerit metus et pharetra.

`

// bootstrapCmd defines the "debug" command used to display diagnostic information for developers, including debug data.
var bootstrapCmd = &cobra.Command{
	Use:          "bootstrap",
	Short:        "Initialize a fresh node. Alias for install, catchup, and start.",
	Long:         "Text",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		httpPkg := new(api.HttpPkg)

		fmt.Print(style.Purple(style.BANNER))
		out, err := glamour.Render(in, "dark")
		if err != nil {
			return err
		}
		fmt.Println(out)

		model := bootstrap.NewModel()
		p := tea.NewProgram(model)
		var msg *app.BootstrapMsg
		go func() {
			for {
				val := <-model.Outside
				switch val.(type) {
				case app.BootstrapMsg:
					msgVal := val.(app.BootstrapMsg)
					msg = &msgVal
				}
			}
		}()

		if _, err := p.Run(); err != nil {
			log.Fatal(err)
		}
		if msg == nil {
			return nil
		}

		log.Warn(style.Yellow.Render(SudoWarningMsg))
		if msg.Install && !algod.IsInstalled() {
			err := algod.Install()
			if err != nil {
				return err
			}
		}

		// Wait for algod
		time.Sleep(10 * time.Second)

		if !algod.IsRunning() {
			log.Fatal("algod is not running")
		}
		// Create the client
		client, err := algod.GetClient("/var/lib/algorand")
		if err != nil {
			return err
		}

		if msg.Catchup {
			network, err := utils.GetNetworkFromDataDir("/var/lib/algorand")
			if err != nil {
				return err
			}
			// Get the latest catchpoint
			catchpoint, _, err := algod.GetLatestCatchpoint(httpPkg, network)
			if err != nil && err.Error() == api.InvalidNetworkParamMsg {
				log.Fatal("This network does not support fast-catchup.")
			} else {
				log.Info(style.Green.Render("Latest Catchpoint: " + catchpoint))
			}

			// Start catchup
			res, _, err := algod.StartCatchup(ctx, client, catchpoint, nil)
			if err != nil {
				log.Fatal(err)
			}
			log.Info(style.Green.Render(res))

		}

		t := new(system.Clock)
		// Fetch the state and handle any creation errors
		state, stateResponse, err := algod.NewStateModel(ctx, client, httpPkg)
		cmdutils.WithInvalidResponsesExplanations(err, stateResponse, cmd.UsageString())
		cobra.CheckErr(err)

		// Construct the TUI Model from the State
		m, err := ui.NewViewportViewModel(state, client)
		cobra.CheckErr(err)

		// Construct the TUI Application
		p = tea.NewProgram(
			m,
			tea.WithAltScreen(),
			tea.WithFPS(120),
		)

		// Watch for State Updates on a separate thread
		// TODO: refactor into context aware watcher without callbacks
		go func() {
			state.Watch(func(status *algod.StateModel, err error) {
				if err == nil {
					p.Send(state)
				}
				if err != nil {
					p.Send(state)
					p.Send(err)
				}
			}, ctx, t)
		}()

		// Execute the TUI Application
		_, err = p.Run()
		if err != nil {
			log.Fatal(err)
		}
		return nil
	},
}
