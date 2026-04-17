import store from '../store';
import { models } from '../constants';
import { getAuthRecord } from '../api';

// SuperAdminRoleID mirrors the backend constant
// (internal/auth/models.go). A user with this numeric role ID bypasses
// permission checks in the UI, matching the server-side behavior.
export const SUPER_ADMIN_ROLE_ID = 1;

// getRoleId resolves the current user's numeric role ID from the first
// source that has it:
//   1. profile.userRole.id        (only useful when the backend populates
//                                  the legacy numeric ID here)
//   2. profile.userRoleId         (same caveat)
//   3. PocketBase auth record's `role` field (this is where the numeric
//      role ID actually lives in the current backend — the nested
//      user_role.id on /api/profile is a PB record ID string and will
//      not match by number).
export function getRoleId(profile) {
  if (profile) {
    if (profile.userRole && Number(profile.userRole.id) > 0) {
      return Number(profile.userRole.id);
    }
    if (Number(profile.userRoleId) > 0) {
      return Number(profile.userRoleId);
    }
  }

  const authRecord = typeof getAuthRecord === 'function' ? getAuthRecord() : null;
  if (authRecord && Number(authRecord.role) > 0) {
    return Number(authRecord.role);
  }

  return 0;
}

export function isSuperAdmin(profile) {
  return getRoleId(profile) === SUPER_ADMIN_ROLE_ID;
}

// getStoreProfile returns the current user profile from the Vuex store,
// or null if it is missing / not yet hydrated.
export function getStoreProfile() {
  const profile = store.getters[models.profile];
  if (!profile || Array.isArray(profile)) {
    return null;
  }
  return profile;
}

export function isCurrentUserSuperAdmin() {
  return isSuperAdmin(getStoreProfile());
}
