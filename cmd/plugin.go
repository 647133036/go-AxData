package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (r *RootCmd) addPluginCmd() {
	pm := r.pluginMgr

	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Plugin management",
		Long:  "Manage AxData plugins.",
	}

	// plugin list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		Run: func(cmd *cobra.Command, args []string) {
			plugins := pm.List()
			if len(plugins) == 0 {
				fmt.Println("No plugins installed.")
				return
			}

			fmt.Printf("%-15s %-15s %-10s %-10s\n", "ID", "NAME", "VERSION", "STATUS")
			fmt.Println(strings.Repeat("-", 55))
			for _, p := range plugins {
				status := "disabled"
				if p.Enabled {
					status = "enabled"
				}
				fmt.Printf("%-15s %-15s %-10s %-10s\n", p.ID, p.Name, p.Version, status)
			}
		},
	})

	// plugin install
	cmd.AddCommand(&cobra.Command{
		Use:   "install <path>",
		Short: "Install a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := pm.Install(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Plugin installed: %s\n", args[0])
			return nil
		},
	})

	// plugin uninstall
	cmd.AddCommand(&cobra.Command{
		Use:   "uninstall <id>",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := pm.Uninstall(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Plugin uninstalled: %s\n", args[0])
			return nil
		},
	})

	// plugin enable
	cmd.AddCommand(&cobra.Command{
		Use:   "enable <id>",
		Short: "Enable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := pm.Enable(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Plugin enabled: %s\n", args[0])
			return nil
		},
	})

	// plugin disable
	cmd.AddCommand(&cobra.Command{
		Use:   "disable <id>",
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := pm.Disable(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Plugin disabled: %s\n", args[0])
			return nil
		},
	})

	// plugin info
	cmd.AddCommand(&cobra.Command{
		Use:   "info <id>",
		Short: "Show plugin details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, ok := pm.Get(args[0])
			if !ok {
				return fmt.Errorf("plugin not found: %s", args[0])
			}

			data, _ := json.MarshalIndent(p, "", "  ")
			fmt.Printf("%s\n", data)
			return nil
		},
	})

	// plugin interfaces - list all interfaces from enabled providers
	cmd.AddCommand(&cobra.Command{
		Use:   "interfaces",
		Short: "List all interfaces from enabled providers",
		Long:  "Display the interface catalog from all enabled providers, including Chinese documentation.",
		Run: func(cmd *cobra.Command, args []string) {
			interfaces := pm.ListInterfaces()
			if len(interfaces) == 0 {
				fmt.Println("No interfaces available.")
				return
			}

			for _, iface := range interfaces {
				menu := strings.Join(iface.MenuPath, " / ")
				fmt.Printf("\n%-30s  %s\n", iface.Name, menu)
				fmt.Printf("  %-30s  %s\n", "中文名称:", iface.DisplayNameZh)
				fmt.Printf("  %-30s  %s\n", "摘要:", iface.SummaryZh)
				fmt.Printf("  %-30s  %s\n", "来源:", iface.SourceCode)
				if len(iface.Parameters) > 0 {
					fmt.Printf("  %-30s\n", "参数:")
					for _, p := range iface.Parameters {
						req := "可选"
						if p.Required {
							req = "必填"
						}
						fmt.Printf("    %-20s %-8s %-10s %s\n", p.Name, p.Type, req, p.Description)
					}
				}
				if len(iface.Fields) > 0 {
					fmt.Printf("  %-30s (%d 个字段)\n", "返回字段:", len(iface.Fields))
				}
			}
		},
	})

	// Add plugin command to root
	r.cmd.AddCommand(cmd)
}
