package principal

import "testing"

func TestEffectiveRole(t *testing.T) {
	tests := []struct {
		name                            string
		p                               Principal
		read, edit, adminProj, adminOrg bool
	}{
		{
			name: "org owner reaches any project without a membership",
			p:    Principal{OrgRole: OrgOwner, ProjectRole: None},
			read: true, edit: true, adminProj: true, adminOrg: true,
		},
		{
			name: "org admin reaches any project without a membership",
			p:    Principal{OrgRole: OrgAdmin, ProjectRole: None},
			read: true, edit: true, adminProj: true, adminOrg: true,
		},
		{
			// The common case, and the one that must not silently widen: an org
			// member sees only the projects they were added to.
			name: "org member without a project membership has nothing",
			p:    Principal{OrgRole: OrgMember, ProjectRole: None},
			read: false, edit: false, adminProj: false, adminOrg: false,
		},
		{
			name: "org member as project viewer may read but not edit",
			p:    Principal{OrgRole: OrgMember, ProjectRole: ProjectViewer},
			read: true, edit: false, adminProj: false, adminOrg: false,
		},
		{
			name: "org member as project editor may edit but not administer",
			p:    Principal{OrgRole: OrgMember, ProjectRole: ProjectEditor},
			read: true, edit: true, adminProj: false, adminOrg: false,
		},
		{
			name: "project admin does not gain org administration",
			p:    Principal{OrgRole: OrgMember, ProjectRole: ProjectAdmin},
			read: true, edit: true, adminProj: true, adminOrg: false,
		},
		{
			name: "zero value grants nothing",
			p:    Principal{},
			read: false, edit: false, adminProj: false, adminOrg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.CanRead(); got != tt.read {
				t.Errorf("CanRead() = %v, want %v", got, tt.read)
			}
			if got := tt.p.CanEdit(); got != tt.edit {
				t.Errorf("CanEdit() = %v, want %v", got, tt.edit)
			}
			if got := tt.p.CanAdminProject(); got != tt.adminProj {
				t.Errorf("CanAdminProject() = %v, want %v", got, tt.adminProj)
			}
			if got := tt.p.CanAdminOrg(); got != tt.adminOrg {
				t.Errorf("CanAdminOrg() = %v, want %v", got, tt.adminOrg)
			}
		})
	}
}
