// Generic, audience-agnostic keys. Lives in frontend-shared so every
// app (user-frontend, admin-frontend) gets them. App-specific keys
// (auth.*, admin.*, trading.*, etc.) belong in the app's own locale
// files and are deep-merged over this at i18n init time.
//
// Step 2 seeds this with the error keys that shared/utils/errorHandler
// looks up. Further generic keys (common.*, validation.*) will be
// migrated from the existing locale files in Steps 3 and 4, once the
// per-app locale files exist and the split can be audited end-to-end.

export default {
  errors: {
    networkError: 'Network error. Please check your connection.',
    requestTimeout: 'Request timed out. Please try again.',
    invalidRequest: 'Invalid request.',
    sessionExpired: 'Your session has expired. Please log in again.',
    accessDenied: 'Access denied.',
    notFound: 'Not found.',
    validationError: 'Please check your input and try again.',
    tooManyRequests: 'Too many requests. Please slow down.',
    serverError: 'Server error. Please try again later.',
    backendServiceUnavailable: 'Service temporarily unavailable.',
    serviceUnavailable: 'Service temporarily unavailable.',
    gatewayTimeout: 'Gateway timeout. Please try again.',
    defaultError: 'Something went wrong. Please try again.',
  },
};
