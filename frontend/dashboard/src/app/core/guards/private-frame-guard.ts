import { DOCUMENT } from '@angular/common';
import { inject } from '@angular/core';
import { CanActivateFn, PRIMARY_OUTLET, Router } from '@angular/router';

// Public shells may be embedded; private client-side navigation must stay blocked.
export const privateFrameGuard: CanActivateFn = (_route, state) => {
    const view = inject(DOCUMENT).defaultView;
    if (!view || view.self === view.top) return true;
    const segments = inject(Router).parseUrl(state.url).root.children[PRIMARY_OUTLET]?.segments ?? [];
    return (segments[0]?.path === 'share' || segments[0]?.path === 'qr-share') && !!segments[1]?.path;
};
