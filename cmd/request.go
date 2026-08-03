package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/electkismet/axdata-go/core/collector"
	"github.com/spf13/cobra"
)

func newRequestCmd(r *RootCmd) *cobra.Command {
	var sourceName string
	var interfaceName string
	var params map[string]string

	cmd := &cobra.Command{
		Use:   "request [source]",
		Short: "Test a source request",
		Long:  "Make a test request to a data source and display results.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			iface := make(map[string]interface{})
			for k, v := range params {
				iface[k] = v
			}
			r.runRequest(ctx, args[0], sourceName, interfaceName, iface)
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "Source name")
	cmd.Flags().StringVar(&interfaceName, "interface", "quote", "Interface name")
	cmd.Flags().StringToStringVarP(&params, "params", "p", nil, "Request parameters (key=value)")

	return cmd
}

func (r *RootCmd) runRequest(ctx context.Context, sourceID, sourceName, interfaceName string, params map[string]interface{}) {
	if sourceName == "" {
		sourceName = sourceID
	}

	adapter := collector.Lookup(sourceName)
	if adapter == nil {
		fmt.Printf("Unknown source: %s\n", sourceName)
		fmt.Printf("Available sources: %v\n", collector.List())
		return
	}

	fmt.Printf("Requesting data from source: %s\n", adapter.Name())
	fmt.Printf("Interface: %s\n", interfaceName)
	fmt.Printf("Description: %s\n", adapter.Description())

	reqParams := map[string]interface{}{
		"interface": interfaceName,
	}
	for k, v := range params {
		reqParams[k] = v
	}

	data, err := adapter.Request(ctx, reqParams)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}

	if len(data) == 0 {
		fmt.Println("No data returned")
		return
	}

	fmt.Printf("Results: %d records\n\n", len(data))
	for i, record := range data {
		if i >= 10 {
			fmt.Printf("... (%d more records)\n", len(data)-10)
			break
		}
		jsonData, _ := json.Marshal(record)
		fmt.Printf("  [%d] %s\n", i+1, strings.TrimSpace(string(jsonData)))
	}
}
