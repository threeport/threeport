package installer

import (
	"fmt"
	"path/filepath"
	"slices"

	. "github.com/dave/jennifer/jen"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	sdk "github.com/threeport/threeport/pkg/sdk/v0"
	"github.com/threeport/threeport/pkg/sdk/v0/gen"
	"github.com/threeport/threeport/pkg/sdk/v0/util"
)

// GenInstallerDatabasePolicyTest generates the test of a module's database
// network policy: that it admits the module's API server pods from the
// module's namespace to the control plane's database, and nothing else.
func GenInstallerDatabasePolicyTest(gen *gen.Generator, sdkConfig *sdk.SdkConfig) error {
	f := NewFile("v0")
	f.HeaderComment(sdk.HeaderCommentGenNoEdit)

	f.ImportAlias("github.com/threeport/threeport/pkg/threeport-installer/v0", "tp_installer")
	f.ImportAlias("github.com/threeport/threeport/pkg/networkpolicytest/v0", "networkpolicytest")

	f.Comment("TestOnlyTheModuleApiServerReachesTheDatabase checks the network policy")
	f.Comment("the installer creates in the control plane's namespace: it admits the")
	f.Comment("module API server's pods, whose db-init and database-migrator init")
	f.Comment("containers connect to the module's database too, from the module's")
	f.Comment("namespace and on the SQL port, and keeps every other pod out.")
	f.Func().Id("TestOnlyTheModuleApiServerReachesTheDatabase").Params(
		Id("t").Op("*").Qual("testing", "T"),
	).Block(
		Id("namespace").Op(":=").Op("&").Qual(
			"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured",
			"Unstructured",
		).Values(),
		Id("namespace").Dot("SetName").Call(Id("defaultNamespace")),
		Id("namespace").Dot("SetUID").Call(
			Qual("k8s.io/apimachinery/pkg/types", "UID").Call(Lit("module-namespace-uid")),
		),
		Line(),

		Id("policy").Op(":=").Qual(
			"github.com/threeport/threeport/pkg/threeport-installer/v0",
			"ModuleDatabasePolicy",
		).Call(
			Line().Id("defaultThreeportNamespace"),
			Line().Id("namespace"),
			Line().Id("apiServerName"),
			Line().Id("apiServerPodLabels").Call(),
			Line(),
		),
		Qual(
			"github.com/threeport/threeport/pkg/networkpolicytest/v0",
			"AssertModuleDatabasePolicy",
		).Call(
			Line().Id("t"),
			Line().Id("policy"),
			Line().Id("defaultThreeportNamespace"),
			Line().Id("defaultNamespace"),
			Line().Id("apiServerPodLabels").Call(),
			Line(),
		),
	)

	// write code to file if not excluded by SDK config
	genFilepath := filepath.Join(
		"pkg",
		"installer",
		"v0",
		"database_policy_gen_test.go",
	)
	if slices.Contains(sdkConfig.ExcludeFiles, genFilepath) {
		cli.Info(fmt.Sprintf("source code generation skipped for %s", genFilepath))
	} else {
		_, err := util.WriteCodeToFile(f, genFilepath, true)
		if err != nil {
			return fmt.Errorf("failed to write generated code to file %s: %w", genFilepath, err)
		}
		cli.Info(fmt.Sprintf("source code for installer database policy test written to %s", genFilepath))
	}

	return nil
}
