package agentdeploy

import (
	"context"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"open-cluster-management.io/addon-framework/pkg/addonmanager/constants"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/basecontroller/factory"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workapiv1 "open-cluster-management.io/api/work/v1"
)

type defaultSyncer struct {
	applyWork func(ctx context.Context, appliedType string,
		work *workapiv1.ManifestWork, addon *addonapiv1alpha1.ManagedClusterAddOn) (*workapiv1.ManifestWork, error)

	getWorkByAddon func(addonName, addonNamespace string) ([]*workapiv1.ManifestWork, error)

	deleteWork func(ctx context.Context, workNamespace, workName string) error

	agentAddon agent.AgentAddon
}

func (s *defaultSyncer) sync(ctx context.Context,
	syncCtx factory.SyncContext,
	cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (*addonapiv1alpha1.ManagedClusterAddOn, error) {
	installMode := constants.InstallModeDefault
	deployWorkNamespace := addon.Namespace

	var errs []error

	if !addon.DeletionTimestamp.IsZero() {
		return addon, nil
	}

	// waiting for the addon to be deleted when cluster is deleting.
	// TODO: consider to delete addon in this scenario.
	if !cluster.DeletionTimestamp.IsZero() {
		return addon, nil
	}

	deployWorks, _, err := buildManifestWorks(ctx, s.agentAddon, installMode, deployWorkNamespace, cluster, addon)
	if err != nil {
		return addon, err
	}

	currentWorks, err := s.getWorkByAddon(addon.Name, addon.Namespace)
	if err != nil {
		return addon, err
	}

	requiredWorkNames := sets.NewString()
	for _, work := range deployWorks {
		requiredWorkNames.Insert(work.Name)
	}
	for _, work := range currentWorks {
		if requiredWorkNames.Has(work.Name) {
			continue
		}
		err = s.deleteWork(ctx, deployWorkNamespace, work.Name)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, deployWork := range deployWorks {
		_, err = s.applyWork(ctx, constants.AddonManifestApplied, deployWork, addon)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return addon, utilerrors.NewAggregate(errs)
}
