# Badge

Badge is a small status indicator for another element.

## Import

```typescript
import { BadgeModule, Badge, BadgeDirective } from '@openng/optimus-ui/badge';
```

## Basic

Content of the badge is specified using the value property.

```typescript
import { Component } from '@angular/core';
import { BadgeModule } from '@openng/optimus-ui/badge';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-badge value="2" />
        </div>
    `,
    standalone: true,
    imports: [BadgeModule]
})
export class BadgeBasicDemo {}
```

## Severity

Severity defines the color of the badge, possible values are success , info , warn and danger

```typescript
import { Component } from '@angular/core';
import { BadgeModule } from '@openng/optimus-ui/badge';

@Component({
    template: `
        <div class="card flex justify-center gap-2">
            <p-badge value="2" />
            <p-badge value="6" severity="secondary" />
            <p-badge value="8" severity="success" />
            <p-badge value="4" severity="info" />
            <p-badge value="9" severity="warn" />
            <p-badge value="3" severity="danger" />
            <p-badge value="5" severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [BadgeModule]
})
export class BadgeSeverityDemo {}
```

## Size

Badge sizes are adjusted with the badgeSize property that accepts small , large and xlarge as the possible alternatives to the default size. Currently sizes only apply to component mode.

```typescript
import { Component } from '@angular/core';
import { BadgeModule } from '@openng/optimus-ui/badge';

@Component({
    template: `
        <div class="card flex justify-center gap-1 items-end">
            <p-badge value="8" badgeSize="xlarge" severity="success" />
            <p-badge value="6" badgeSize="large" severity="warn" />
            <p-badge value="4" severity="info" />
            <p-badge value="2" badgeSize="small" />
        </div>
    `,
    standalone: true,
    imports: [BadgeModule]
})
export class BadgeSizeDemo {}
```

## Overlay

A badge can be added to any element by encapsulating the content with the OverlayBadge component.

```typescript
import { Component } from '@angular/core';
import { OverlayBadgeModule } from '@openng/optimus-ui/overlaybadge';

@Component({
    template: `
        <div class="card flex flex-wrap justify-center gap-6">
            <p-overlaybadge value="2">
                <i class="pi pi-bell" style="font-size: 2rem"></i>
            </p-overlaybadge>
            <p-overlaybadge value="4" severity="danger">
                <i class="pi pi-calendar" style="font-size: 2rem"></i>
            </p-overlaybadge>
            <p-overlaybadge severity="danger">
                <i class="pi pi-envelope" style="font-size: 2rem"></i>
            </p-overlaybadge>
        </div>
    `,
    standalone: true,
    imports: [OverlayBadgeModule]
})
export class BadgeOverlayDemo {}
```

## Button

Buttons have built-in support for badges to display a badge inline.

```typescript
import { Component } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-button label="Emails" icon="pi pi-bell" label="Notifications" badge="2" />
            <p-button label="Inbox" icon="pi pi-inbox" badge="2" badgeSeverity="contrast" outlined />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule]
})
export class BadgeButtonDemo {}
```

## Accessibility

Screen Reader Badge does not include any roles and attributes by default, any attribute is passed to the root element so aria roles and attributes can be added if required. If the badges are dynamic, aria-live may be utilized as well. In case badges need to be tabbable, tabIndex can be added to implement custom key handlers. Keyboard Support Component does not include any interactive elements.

## Directive

Content of the badge is specified using the value property.

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="card flex justify-center">
            <i class="pi pi-bell !text-3xl" pBadge value="2"></i>
        </div>
    `,
    standalone: true,
    imports: []
})
export class BadgeDirectiveDemo {}
```

## Position

A Badge can be positioned at the top right corner of an element by adding p-overlay-badge style class to the element and embedding the badge inside.

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="card flex justify-center">
            <i class="pi pi-bell mr-6 p-text-secondary" pBadge style="font-size: 2rem" value="2"></i>
            <i class="pi pi-calendar mr-6 p-text-secondary" pBadge style="font-size: 2rem" [value]="'10+'" severity="danger"></i>
            <i class="pi pi-envelope p-text-secondary" pBadge style="font-size: 2rem" severity="danger"></i>
        </div>
    `,
    standalone: true,
    imports: []
})
export class BadgePositionDemo {}
```

## API

### Badge

Selector: `p-badge` · Component, standalone

Badge is a small status indicator for another element.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `badgeDisabled` | `boolean` | `false` | When specified, disables the component. |
| `badgeSize` | `"large" \| "small" \| "xlarge"` | - | Size of the badge, valid options are "large" and "xlarge". |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pt` | `BadgePassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `severity` | `"success" \| "info" \| "warn" \| "danger" \| "secondary" \| "contrast"` | - | Severity type of the badge. |
| `size` | `"large" \| "small" \| "xlarge"` | - | Size of the badge, valid options are "large" and "xlarge". |
| `styleClass` | `string \| undefined` | - | Class of the element. **Deprecated**: since v20.0.0, use `class` instead. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `string \| number \| null \| undefined` | - | Value to display inside the badge. |

### BadgeDirective

Selector: `[pBadge]` · Directive, standalone

Badge Directive is directive usage of badge component.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `badgeDisabled` | `boolean` | - | When specified, disables the component. |
| `badgeSize` | `"large" \| "small" \| "xlarge"` | - | Size of the badge, valid options are "large" and "xlarge". |
| `badgeStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the element. |
| `badgeStyleClass` | `string` | - | Class of the element. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pBadgePT` | `BadgePassThrough` | - | Used to pass attributes to DOM elements inside the Badge component. |
| `pBadgeUnstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `pt` | `any` | - | Used to pass attributes to DOM elements inside the component. |
| `ptBadgeDirective` | `BadgePassThrough` | - | Used to pass attributes to DOM elements inside the Badge component. **Deprecated**: use pBadgePT instead. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `severity` | `"success" \| "info" \| "warn" \| "danger" \| "secondary" \| "contrast"` | - | Severity type of the badge. |
| `size` | `"large" \| "small" \| "xlarge"` | - | Size of the badge, valid options are "large" and "xlarge". **Deprecated**: use badgeSize instead. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `string \| number` | - | Value to display inside the badge. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the root's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-badge` | Class name of the root element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `badge.border.radius` | `--p-badge-border-radius` | Border radius of root |
| `badge.padding` | `--p-badge-padding` | Padding of root |
| `badge.font.size` | `--p-badge-font-size` | Font size of root |
| `badge.font.weight` | `--p-badge-font-weight` | Font weight of root |
| `badge.min.width` | `--p-badge-min-width` | Min width of root |
| `badge.height` | `--p-badge-height` | Height of root |
| `badge.dot.size` | `--p-badge-dot-size` | Size of dot |
| `badge.sm.font.size` | `--p-badge-sm-font-size` | Font size of sm |
| `badge.sm.min.width` | `--p-badge-sm-min-width` | Min width of sm |
| `badge.sm.height` | `--p-badge-sm-height` | Height of sm |
| `badge.lg.font.size` | `--p-badge-lg-font-size` | Font size of lg |
| `badge.lg.min.width` | `--p-badge-lg-min-width` | Min width of lg |
| `badge.lg.height` | `--p-badge-lg-height` | Height of lg |
| `badge.xl.font.size` | `--p-badge-xl-font-size` | Font size of xl |
| `badge.xl.min.width` | `--p-badge-xl-min-width` | Min width of xl |
| `badge.xl.height` | `--p-badge-xl-height` | Height of xl |
| `badge.primary.background` | `--p-badge-primary-background` | Background of primary |
| `badge.primary.color` | `--p-badge-primary-color` | Color of primary |
| `badge.secondary.background` | `--p-badge-secondary-background` | Background of secondary |
| `badge.secondary.color` | `--p-badge-secondary-color` | Color of secondary |
| `badge.success.background` | `--p-badge-success-background` | Background of success |
| `badge.success.color` | `--p-badge-success-color` | Color of success |
| `badge.info.background` | `--p-badge-info-background` | Background of info |
| `badge.info.color` | `--p-badge-info-color` | Color of info |
| `badge.warn.background` | `--p-badge-warn-background` | Background of warn |
| `badge.warn.color` | `--p-badge-warn-color` | Color of warn |
| `badge.danger.background` | `--p-badge-danger-background` | Background of danger |
| `badge.danger.color` | `--p-badge-danger-color` | Color of danger |
| `badge.contrast.background` | `--p-badge-contrast-background` | Background of contrast |
| `badge.contrast.color` | `--p-badge-contrast-color` | Color of contrast |
