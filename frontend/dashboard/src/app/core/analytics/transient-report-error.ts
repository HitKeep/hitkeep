import { HttpErrorResponse } from '@angular/common/http';

export function isTransientReportError(error: unknown): boolean {
    return error instanceof HttpErrorResponse && (error.status === 0 || error.status === 408 || error.status === 429 || error.status >= 500);
}
