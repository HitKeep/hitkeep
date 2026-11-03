export interface AgentOperation {
    id: string;
    method: string;
    path: string;
    summary: string;
    tags: string[];
    parameters: unknown;
    requestBody?: unknown;
}

const methods = new Set(['get', 'post', 'put', 'patch', 'delete']);
const credentialPath = /\/(?:auth|oauth|login|logout|setup|api[-_]clients|share|takeout|webhooks?|password|passkeys?|mfa|totp|recovery|sso|social|unsubscribe|exports?|avatar|favicon|realtime)(?:\/|$)|\{[^}]*token[^}]*\}/i;
const credentialKey = /password|secret|private_key|recovery_codes?|authorization|(?:^|_)(?:access_token|refresh_token|id_token|api_key|token)$/i;

function record(value: unknown): Record<string, unknown> | null {
    return value !== null && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

export function agentOperations(document: unknown): AgentOperation[] {
    const paths = record(record(document)?.['paths']);
    if (!paths) return [];
    return Object.entries(paths).flatMap(([path, value]) => {
        const item = record(value);
        if (!item || !/^\/api\/[\w/{}.-]+$/.test(path) || path.includes('..') || path.includes('//') || credentialPath.test(path) || path === '/api/user/teams/active') return [];
        return Object.entries(item).flatMap(([method, value]) => {
            const operation = record(value);
            if (!methods.has(method) || !operation || operation['x-internal'] === true || item['x-internal'] === true) return [];
            if (path.endsWith('/asset') && method !== 'delete') return [];
            const requestContent = record(record(operation['requestBody'])?.['content']);
            if (requestContent && !Object.keys(requestContent).some((type) => type === 'application/json' || type.endsWith('+json'))) return [];
            const tags = Array.isArray(operation['tags']) ? operation['tags'].filter((tag): tag is string => typeof tag === 'string') : [];
            if (tags.some((tag) => ['Auth', 'Share', 'Takeout', 'Webhooks'].includes(tag))) return [];
            const parameters = [...(Array.isArray(item['parameters']) ? item['parameters'] : []), ...(Array.isArray(operation['parameters']) ? operation['parameters'] : [])];
            if (
                parameters.some((value) => {
                    const parameter = record(value);
                    return parameter?.['in'] === 'header' && parameter['required'] === true;
                })
            )
                return [];
            const responses = record(operation['responses']);
            const success = record(responses?.['200']);
            const content = record(success?.['content']);
            if (content && !Object.keys(content).some((type) => type === 'application/json' || type.endsWith('+json'))) return [];
            return [
                {
                    id: `${method.toUpperCase()} ${path}`,
                    method: method.toUpperCase(),
                    path,
                    summary: typeof operation['summary'] === 'string' ? operation['summary'] : `${method.toUpperCase()} ${path}`,
                    tags,
                    parameters,
                    ...(operation['requestBody'] ? { requestBody: operation['requestBody'] } : {})
                }
            ];
        });
    });
}

export function agentRequest(operation: AgentOperation, value: unknown): { method: string; url: string; body?: unknown } {
    const input = record(value);
    if (!input) throw new Error('Invalid action input');
    const parameters = record(input['path_parameters']) ?? {};
    const path = operation.path.replace(/\{([^}]+)\}/g, (_, name: string) => {
        const value = parameters[name];
        if (typeof value !== 'string' || !value || value.length > 2048 || value === '.' || value === '..' || /[/%\\?#]/.test(value)) throw new Error('Invalid path parameter');
        return encodeURIComponent(value);
    });
    const query = input['query'] === undefined ? {} : record(input['query']);
    if (!query) throw new Error('Invalid query');
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
        if (credentialKey.test(key) || !['string', 'number', 'boolean'].includes(typeof value) || (typeof value === 'number' && !Number.isFinite(value))) throw new Error('Invalid query parameter');
        search.set(key, String(value));
    }
    let body = input['body'];
    if (body !== undefined) {
        const json = JSON.stringify(body);
        if (!json || new TextEncoder().encode(json).byteLength > 65536) throw new Error('Invalid request body');
        body = JSON.parse(json);
    }
    const suffix = search.toString();
    return { method: operation.method, url: path + (suffix ? `?${suffix}` : ''), ...(body !== undefined ? { body } : {}) };
}

export function redactAgentResult(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(redactAgentResult);
    const object = record(value);
    if (!object) return value;
    return Object.fromEntries(Object.entries(object).map(([key, value]) => [key, credentialKey.test(key) ? '[redacted]' : redactAgentResult(value)]));
}
