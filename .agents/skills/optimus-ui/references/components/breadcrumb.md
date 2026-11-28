# Breadcrumb

Breadcrumb provides contextual information about page hierarchy.

## Import

```typescript
import { BreadcrumbModule, Breadcrumb } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';
```

## Basic

Breadcrumb provides contextual information about page hierarchy.

```typescript
import { Component, OnInit } from '@angular/core';
import { BreadcrumbModule } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-breadcrumb [model]="items" [home]="home" />
        </div>
    `,
    standalone: true,
    imports: [BreadcrumbModule]
})
export class BreadcrumbBasicDemo implements OnInit {
    items: MenuItem[] | undefined;
    home: MenuItem | undefined;

    ngOnInit() {
        this.items = [{ label: 'Electronics' }, { label: 'Computer' }, { label: 'Accessories' }, { label: 'Keyboard' }, { label: 'Wireless' }];
        this.home = { icon: 'pi pi-home' };
    }
}
```

## Template

Custom content can be placed inside the items using the item template. The divider between the items has its own separator template.

```typescript
import { Component, OnInit } from '@angular/core';
import { BreadcrumbModule } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-breadcrumb [model]="items" [home]="home">
                <ng-template #item let-item>
                    <a class="cursor-pointer" [routerLink]="item.url">
                        <i [class]="item.icon"></i>
                    </a>
                </ng-template>
                <ng-template #separator> / </ng-template>
            </p-breadcrumb>
        </div>
    `,
    standalone: true,
    imports: [BreadcrumbModule]
})
export class BreadcrumbTemplateDemo implements OnInit {
    items: MenuItem[] | undefined;
    home: MenuItem | undefined;

    ngOnInit() {
        this.items = [{ icon: 'pi pi-sitemap' }, { icon: 'pi pi-book' }, { icon: 'pi pi-wallet' }, { icon: 'pi pi-shopping-bag' }, { icon: 'pi pi-calculator' }];
        this.home = { icon: 'pi pi-home' };
    }
}
```

## Router

Menu items support navigation via routerLink, programmatic routing using commands, or external URLs.

```typescript
import { Component } from '@angular/core';
import { BreadcrumbModule } from '@openng/optimus-ui/breadcrumb';
import { MenuItem } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-breadcrumb [home]="home" [model]="items" />
        </div>
    `,
    standalone: true,
    imports: [BreadcrumbModule]
})
export class BreadcrumbRouterDemo {
    items: MenuItem[] = [{ label: 'Components' }, { label: 'Form' }, { label: 'InputText', routerLink: '/inputtext' }];
    home: MenuItem = { icon: 'pi pi-home', routerLink: '/' };
}
```

## Accessibility

Screen Reader Breadcrumb uses the nav element and since any attribute is passed to the root implicitly aria-labelledby or aria-label can be used to describe the component. Inside an ordered list is used where the list item separators have aria-hidden to be able to ignored by the screen readers. If the last link represents the current route, aria-current is added with "page" as the value. The home item renders an icon, so when it has no label the link is named with the homeAriaLabel property. When homeAriaLabel is not defined either, the aria.home key of the locale configuration is used as the default. Keyboard Support No special keyboard interaction is needed, all menuitems are focusable based on the page tab sequence.

## API

### Breadcrumb

Selector: `p-breadcrumb` · Component, standalone

Breadcrumb provides contextual information about page hierarchy.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `home` | `MenuItem \| undefined` | - | MenuItem configuration for the home icon. |
| `homeAriaLabel` | `string \| undefined` | - | Defines a string that labels the home icon for accessibility. Defaults to the `aria.home` translation when the home item has no visible label. |
| `model` | `MenuItem[] \| undefined` | - | An array of menuitems. |
| `pt` | `BreadcrumbPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `style` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the component. |
| `styleClass` | `string \| undefined` | - | Style class of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onItemClick` | `BreadcrumbItemClickEvent` | Fired when an item is selected. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-breadcrumb` element.

| Name | Context | Description |
|---|---|---|
| `item` | `BreadcrumbItemTemplateContext` | Custom item template. |
| `separator` | - | Custom separator template. |

### MenuItem

MenuItem provides the following properties. Note that not all of them may be utilized by the tabmenu component.

| Name | Type | Description |
|---|---|---|
| `label?` | `string \| undefined` | Text of the item. |
| `icon?` | `string \| undefined` | Icon of the item. |
| `command?` | `((event: MenuItemCommandEvent) => void) \| undefined` | Callback to execute when item is clicked. |
| `url?` | `string \| undefined` | External link to navigate when item is clicked. |
| `items?` | `MenuItem[] \| undefined` | An array of children menuitems. |
| `expanded?` | `boolean \| undefined` | Visibility of submenu. |
| `disabled?` | `boolean \| undefined` | When set as true, disables the menuitem. |
| `visible?` | `boolean \| undefined` | Whether the dom element of menuitem is created or not. |
| `target?` | `string \| undefined` | Specifies where to open the linked document. |
| `escape?` | `boolean \| undefined` | Whether to escape the label or not. Set to false to display html content. |
| `routerLinkActiveOptions?` | `any` | Configuration for active router link. |
| `separator?` | `boolean \| undefined` | Defines the item as a separator. |
| `badge?` | `string \| undefined` | Value of the badge. |
| `tooltip?` | `string \| undefined` | Tooltip of the item. |
| `tooltipPosition?` | `string \| undefined` | Position of the tooltip item. |
| `badgeStyleClass?` | `string \| undefined` | Style class of the badge. |
| `style?` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the menuitem. |
| `styleClass?` | `string \| undefined` | Style class of the menuitem. |
| `title?` | `string \| undefined` | Tooltip text of the item. |
| `id?` | `string \| undefined` | Identifier of the element. |
| `automationId?` | `any` | Value of HTML data-* attribute. |
| `tabindex?` | `string \| undefined` | Specifies tab order of the item. |
| `routerLink?` | `any` | RouterLink definition for internal navigation. |
| `queryParams?` | `{ [k: string]: any; } \| undefined` | Query parameters for internal navigation via routerLink. |
| `fragment?` | `string \| undefined` | Sets the hash fragment for the URL. |
| `queryParamsHandling?` | `QueryParamsHandling \| undefined` | How to handle query parameters in the router link for the next navigation. One of: merge : Merge new with current parameters. preserve : Preserve current parameters.k. |
| `preserveFragment?` | `boolean \| undefined` | When true, preserves the URL fragment for the next navigation. |
| `skipLocationChange?` | `boolean \| undefined` | When true, navigates without pushing a new state into history. |
| `replaceUrl?` | `boolean \| undefined` | When true, navigates while replacing the current state in history. |
| `iconStyle?` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the item's icon. |
| `iconClass?` | `string \| undefined` | Class of the item's icon. |
| `labelStyle?` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the item's label. |
| `labelClass?` | `string \| undefined` | Class of the item's label. |
| `linkStyle?` | `{ [klass: string]: any; } \| null \| undefined` | Inline style of the item's link. |
| `linkClass?` | `string \| undefined` | Class of the item's link. |
| `state?` | `{ [k: string]: any; } \| undefined` | Developer-defined state that can be passed to any navigation. |
| `tooltipOptions?` | `TooltipOptions \| undefined` | Options of the item's tooltip. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the root's DOM element. |
| `list` | `PassThroughOption<HTMLOListElement, I>` | Used to pass attributes to the list's DOM element. |
| `homeItem` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the home item's DOM element. |
| `item` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the item's DOM element. |
| `itemLink` | `PassThroughOption<HTMLAnchorElement, I>` | Used to pass attributes to the item link's DOM element. |
| `itemIcon` | `PassThroughOption<SVGElement \| HTMLSpanElement, I>` | Used to pass attributes to the item icon's DOM element. |
| `itemLabel` | `PassThroughOption<HTMLSpanElement, I>` | Used to pass attributes to the item label's DOM element. |
| `separator` | `PassThroughOption<HTMLLIElement, I>` | Used to pass attributes to the separator's DOM element. |
| `separatorIcon` | `PassThroughOption<SVGElement, I>` | Used to pass attributes to the separator icon's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-breadcrumb` | Class name of the root element |
| `p-breadcrumb-list` | Class name of the list element |
| `p-breadcrumb-home-item` | Class name of the home item element |
| `p-breadcrumb-separator` | Class name of the separator element |
| `p-breadcrumb-item` | Class name of the item element |
| `p-breadcrumb-item-link` | Class name of the item link element |
| `p-breadcrumb-item-icon` | Class name of the item icon element |
| `p-breadcrumb-item-label` | Class name of the item label element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `breadcrumb.padding` | `--p-breadcrumb-padding` | Padding of root |
| `breadcrumb.background` | `--p-breadcrumb-background` | Background of root |
| `breadcrumb.gap` | `--p-breadcrumb-gap` | Gap of root |
| `breadcrumb.transition.duration` | `--p-breadcrumb-transition-duration` | Transition duration of root |
| `breadcrumb.item.color` | `--p-breadcrumb-item-color` | Color of item |
| `breadcrumb.item.hover.color` | `--p-breadcrumb-item-hover-color` | Hover color of item |
| `breadcrumb.item.border.radius` | `--p-breadcrumb-item-border-radius` | Border radius of item |
| `breadcrumb.item.gap` | `--p-breadcrumb-item-gap` | Gap of item |
| `breadcrumb.item.icon.color` | `--p-breadcrumb-item-icon-color` | Icon color of item |
| `breadcrumb.item.icon.hover.color` | `--p-breadcrumb-item-icon-hover-color` | Icon hover color of item |
| `breadcrumb.item.focus.ring.width` | `--p-breadcrumb-item-focus-ring-width` | Focus ring width of item |
| `breadcrumb.item.focus.ring.style` | `--p-breadcrumb-item-focus-ring-style` | Focus ring style of item |
| `breadcrumb.item.focus.ring.color` | `--p-breadcrumb-item-focus-ring-color` | Focus ring color of item |
| `breadcrumb.item.focus.ring.offset` | `--p-breadcrumb-item-focus-ring-offset` | Focus ring offset of item |
| `breadcrumb.item.focus.ring.shadow` | `--p-breadcrumb-item-focus-ring-shadow` | Focus ring shadow of item |
| `breadcrumb.separator.color` | `--p-breadcrumb-separator-color` | Color of separator |
