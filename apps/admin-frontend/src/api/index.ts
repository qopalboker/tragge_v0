// Barrel for the admin axios instance. Matches the `@/api` import
// path the pre-split admin views rely on (`import { api } from
// '@/api'`). Individual typed clients live in sibling files
// (./users, ./withdrawals, etc.) and import `api` from here.
export { api, getAccessToken, setAccessToken } from './client';
