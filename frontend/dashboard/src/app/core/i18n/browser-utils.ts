const BROWSER_ICON_MAP: Record<string, string> = {
    chrome: 'chrome',
    'mobile chrome': 'mobile-chrome',
    firefox: 'firefox',
    'mobile firefox': 'mobile-firefox',
    safari: 'safari',
    'mobile safari': 'mobile-safari',
    edge: 'edge',
    opera: 'opera',
    'opera touch': 'opera-touch',
    'opera gx': 'opera-gx',
    brave: 'brave',
    vivaldi: 'vivaldi',
    'samsung internet': 'samsung-internet',
    facebook: 'facebook',
    instagram: 'instagram',
    tiktok: 'tiktok',
    twitter: 'twitter',
    linkedin: 'linkedin',
    wechat: 'wechat',
    line: 'line',
    yandex: 'yandex',
    gsa: 'gsa',
    electron: 'electron',
    webkit: 'webkit',
    duckduckgo: 'duckduckgo',
    'chrome webview': 'chrome-webview',
    'huawei browser': 'huawei-browser',
    'miui browser': 'miui-browser',
    qqbrowser: 'qqbrowser',
    ucbrowser: 'ucbrowser',
    quark: 'quark',
    whale: 'whale',
    ecosia: 'ecosia',
    'vivo browser': 'vivo-browser',
    '360': '360',
    silk: 'silk',
    ie: 'ie',
    'android browser': 'android-browser',
    'avast secure browser': 'avast-secure-browser',
    'avg secure browser': 'avg-secure-browser'
};

const DEFAULT_BROWSER_ICON = 'browsers/_default.avif';

export function browserIconUrl(name: string | null | undefined): string {
    const normalized = (name ?? '').trim().toLowerCase();
    const slug = BROWSER_ICON_MAP[normalized];
    if (slug) {
        return `browsers/${slug}.avif`;
    }
    return DEFAULT_BROWSER_ICON;
}
