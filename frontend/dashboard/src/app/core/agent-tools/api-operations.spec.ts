import { describe, expect, it } from 'vitest';
import { agentOperations, agentRequest, redactAgentResult } from './api-operations';

const document = {
    paths: {
        '/api/sites/{id}/goals': {
            get: { summary: 'List goals', tags: ['Goals'] },
            post: { summary: 'Create a goal', tags: ['Goals'], requestBody: { content: { 'application/json': { schema: { type: 'object' } } } } }
        },
        '/api/user/password': { put: { summary: 'Change password', tags: ['User'] } },
        '/api/user/api-clients': { post: { summary: 'Create token', tags: ['User'] } },
        '/api/sites/{id}/share': { post: { summary: 'Create share token', tags: ['Share'] } },
        '/api/webhooks': { post: { summary: 'Create signing secret', tags: ['Webhooks'] } },
        '/api/user/teams/active': { put: { summary: 'Switch team', tags: ['Teams'] } },
        '/api/export': { get: { responses: { '200': { content: { 'text/csv': {} } } } } },
        '//external.example/api': { get: {} },
        '/api/../auth': { get: {} }
    }
};

describe('agent API operations', () => {
    it('reuses declared API operations while keeping credentials and downloads in the trusted UI', () => {
        expect(agentOperations(document).map((operation) => operation.id)).toEqual(['GET /api/sites/{id}/goals', 'POST /api/sites/{id}/goals']);
    });

    it('handles missing or malformed OpenAPI data without exposing operations', () => {
        for (const value of [null, {}, { paths: [] }, { paths: { '/api/sites': null } }]) expect(agentOperations(value)).toEqual([]);
    });

    it('omits protocol operations that require headers the dashboard bridge cannot supply', () => {
        expect(
            agentOperations({
                paths: {
                    '/api/commerce/checkout_sessions': { parameters: [{ in: 'header', name: 'API-Version', required: true }], post: {} },
                    '/api/ucp/checkout-sessions': { post: { parameters: [{ in: 'header', name: 'UCP-Agent', required: true }] } },
                    '/api/sites/{id}/goals': { parameters: [{ in: 'path', name: 'id', required: true }], get: { parameters: [{ in: 'query', name: 'limit' }] } }
                }
            })
        ).toMatchObject([{ id: 'GET /api/sites/{id}/goals', parameters: [{ name: 'id' }, { name: 'limit' }] }]);
    });

    it('keeps tokenized links, binary downloads and live streams in their existing UI flows', () => {
        expect(
            agentOperations({
                paths: {
                    '/api/reports/unsubscribe/{opaque_token}': { get: {} },
                    '/api/sites/{id}/realtime': { get: { 'x-internal': true } },
                    '/api/sites/{id}/export': { get: {} },
                    '/api/user/avatar': { get: {} },
                    '/api/favicon/{domain}': { get: {} },
                    '/api/sites/{id}/qr-codes/{qrID}/asset': { get: {}, put: {}, delete: {} },
                    '/api/upload': { put: { requestBody: { content: { 'multipart/form-data': {} } } } },
                    '/api/internal': { get: { 'x-internal': true } },
                    '/api/sites/{id}/goals': { get: {} }
                }
            }).map((operation) => operation.id)
        ).toEqual(['DELETE /api/sites/{id}/qr-codes/{qrID}/asset', 'GET /api/sites/{id}/goals']);
    });

    it('builds only declared relative requests with encoded path and query values', () => {
        const operation = agentOperations(document)[1];
        expect(agentRequest(operation, { path_parameters: { id: 'site-1' }, query: { q: 'a & b', page: 2 }, body: { name: 'Purchase' } })).toEqual({
            method: 'POST',
            url: '/api/sites/site-1/goals?q=a+%26+b&page=2',
            body: { name: 'Purchase' }
        });
    });

    it('rejects missing, traversal and encoded path separators before any request', () => {
        const operation = agentOperations(document)[0];
        for (const id of ['', '.', '..', '../other', 'a/b', '%2fadmin', 'a\\b', 'a?b', 'a#b']) {
            expect(() => agentRequest(operation, { path_parameters: { id } })).toThrow();
        }
        expect(() => agentRequest(operation, {})).toThrow();
    });

    it('rejects credential query parameters, nested query values and oversized JSON', () => {
        const operation = agentOperations(document)[1];
        for (const query of [{ access_token: 'secret' }, { authorization: 'secret' }, { nested: { value: 1 } }]) {
            expect(() => agentRequest(operation, { path_parameters: { id: 'site-1' }, query })).toThrow();
        }
        expect(() => agentRequest(operation, { path_parameters: { id: 'site-1' }, body: { name: 'x'.repeat(65536) } })).toThrow();
    });

    it('freezes the request body before asking the user to approve it', () => {
        const body = { name: 'Approved name' };
        const request = agentRequest(agentOperations(document)[1], { path_parameters: { id: 'site-1' }, body });
        body.name = 'Different name';
        expect(request.body).toEqual({ name: 'Approved name' });
    });

    it('removes recoverable credentials from nested API results without hiding analytics counts', () => {
        expect(redactAgentResult({ access_token: 'secret', entries: [{ signing_secret: 'secret', name: 'Goal' }], tokens_used: 42, token_hint: '…abcd' })).toEqual({
            access_token: '[redacted]',
            entries: [{ signing_secret: '[redacted]', name: 'Goal' }],
            tokens_used: 42,
            token_hint: '…abcd'
        });
    });
});
