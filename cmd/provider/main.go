/*
Copyright 2023 Upbound Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/upbound/provider-upbound/apis"
	"github.com/upbound/provider-upbound/apis/v1alpha1"
	upbound "github.com/upbound/provider-upbound/internal/controller"
	"github.com/upbound/provider-upbound/internal/controller/features"
)

func main() {
	var (
		app            = kingpin.New(filepath.Base(os.Args[0]), "Upbound support for Crossplane.").DefaultEnvars()
		debug          = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		leaderElection = app.Flag("leader-election", "Use leader election for the controller manager.").Short('l').Default("false").OverrideDefaultFromEnvar("LEADER_ELECTION").Bool()

		syncInterval     = app.Flag("sync", "How often all resources will be double-checked for drift from the desired state.").Short('s').Default("1h").Duration()
		pollInterval     = app.Flag("poll", "How often individual resources will be checked for drift from the desired state").Default("1m").Duration()
		maxReconcileRate = app.Flag("max-reconcile-rate", "The global maximum rate per second at which resources may checked for drift from the desired state.").Default("10").Int()

		namespace                  = app.Flag("namespace", "Namespace used to set as default scope in default secret store config.").Default("crossplane-system").Envar("POD_NAMESPACE").String()
		enableExternalSecretStores = app.Flag("enable-external-secret-stores", "Enable support for ExternalSecretStores.").Default("false").Envar("ENABLE_EXTERNAL_SECRET_STORES").Bool()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	zl := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zl.WithName("provider-upbound"))
	if *debug {
		// The controller-runtime runs with a no-op logger by default. It is
		// *very* verbose even at info level, so we only provide it a real
		// logger when we're running in debug mode.
		ctrl.SetLogger(zl)
	}

	cfg, err := ctrl.GetConfig()
	kingpin.FatalIfError(err, "Cannot get API server rest config")

	mgr, err := ctrl.NewManager(ratelimiter.LimitRESTConfig(cfg, *maxReconcileRate), ctrl.Options{
		SyncPeriod: syncInterval,

		// controller-runtime uses both ConfigMaps and Leases for leader
		// election by default. Leases expire after 15 seconds, with a
		// 10 second renewal deadline. We've observed leader loss due to
		// renewal deadlines being exceeded when under high load - i.e.
		// hundreds of reconciles per second and ~200rps to the API
		// server. Switching to Leases only and longer leases appears to
		// alleviate this.
		LeaderElection:             *leaderElection,
		LeaderElectionID:           "crossplane-leader-election-provider-upbound",
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaseDuration:              func() *time.Duration { d := 60 * time.Second; return &d }(),
		RenewDeadline:              func() *time.Duration { d := 50 * time.Second; return &d }(),
	})
	kingpin.FatalIfError(err, "Cannot create controller manager")
	kingpin.FatalIfError(apis.AddToScheme(mgr.GetScheme()), "Cannot add Upbound APIs to scheme")

	o := controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: *maxReconcileRate,
		PollInterval:            *pollInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
		Features:                &feature.Flags{},
	}

	if *enableExternalSecretStores {
		o.Features.Enable(features.EnableAlphaExternalSecretStores)
		log.Info("Alpha feature enabled", "flag", features.EnableAlphaExternalSecretStores)

		// Ensure default store config exists.
		kingpin.FatalIfError(resource.Ignore(kerrors.IsAlreadyExists, mgr.GetClient().Create(context.Background(), &v1alpha1.StoreConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: v1alpha1.StoreConfigSpec{
				// NOTE(turkenh): We only set required spec and expect optional
				// ones to properly be initialized with CRD level default values.
				SecretStoreConfig: xpv1.SecretStoreConfig{
					DefaultScope: *namespace,
				},
			},
		})), "cannot create default store config")
	}

	kingpin.FatalIfError(upbound.Setup(mgr, o), "Cannot setup Upbound controllers")
	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}
