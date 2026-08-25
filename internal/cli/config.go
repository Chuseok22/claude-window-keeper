package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Chuseok22/claude-window-keeper/internal/config"
)

func newConfigCmd() *cobra.Command {
	text := localizedText()
	cmd := &cobra.Command{
		Use:     "config",
		Aliases: []string{"c", "cfg"},
		Short:   text.configShort,
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(newConfigInitCmd(), newConfigPathCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var force bool
	text := localizedText()
	cmd := &cobra.Command{
		Use:     "init",
		Aliases: []string{"i"},
		Short:   text.configInitShort,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.WriteDefault(force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, text.configInitForce)
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	text := localizedText()
	return &cobra.Command{
		Use:     "path",
		Aliases: []string{"p"},
		Short:   text.configPathShort,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}
