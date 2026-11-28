# Accordion

Accordion groups a collection of contents in tabs.

## Import

```typescript
import { AccordionModule, Accordion, AccordionPanel, AccordionHeader, AccordionContent } from '@openng/optimus-ui/accordion';
```

## Basic

Accordion is defined using AccordionPanel , AccordionHeader and AccordionContent components. Each AccordionPanel must contain a unique value property to specify the active item.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-accordion value="0">
                <p-accordion-panel value="0">
                    <p-accordion-header>Header I</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
                            consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est
                            laborum.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="1">
                    <p-accordion-header>Header II</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="2">
                    <p-accordion-header>Header III</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule]
})
export class AccordionBasicDemo {}
```

## Dynamic

AccordionPanel can be generated dynamically using the standard &#64;for block.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';

@Component({
    template: `
        <div class="card">
            <p-accordion [value]="['0']">
                @for (tab of tabs; track tab.title) {
                    <p-accordion-panel [value]="tab.value">
                        <p-accordion-header>{{ tab.title }}</p-accordion-header>
                        <p-accordion-content>
                            <p class="m-0">{{ tab.content }}</p>
                        </p-accordion-content>
                    </p-accordion-panel>
                }
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule]
})
export class AccordionDynamicDemo {
    tabs: any[];
}
```

## Multiple

Only one tab at a time can be active by default, enabling multiple property changes this behavior to allow multiple tabs. In this case activeIndex needs to be an array.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';

@Component({
    template: `
        <div class="card flex justify-content-center">
            <p-accordion [value]="['0']" [multiple]="true">
                <p-accordion-panel value="0">
                    <p-accordion-header>Header I</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
                            consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est
                            laborum.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="1">
                    <p-accordion-header>Header II</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="2">
                    <p-accordion-header>Header III</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint occaecati cupiditate non provident, similique sunt in
                            culpa qui officia deserunt mollitia animi, id est laborum et dolorum fuga. Et harum quidem rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio cumque nihil impedit quo minus.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule]
})
export class AccordionMultipleDemo {}
```

## Disabled

Enabling disabled property of an AccordionTab prevents user interaction.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';

@Component({
    template: `
        <div class="card flex justify-content-center">
            <p-accordion [value]="['0']" [multiple]="true">
                <p-accordion-panel value="0">
                    <p-accordion-header>Header I</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
                            consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est
                            laborum.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="1">
                    <p-accordion-header>Header II</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="2">
                    <p-accordion-header>Header III</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint occaecati cupiditate non provident, similique sunt in
                            culpa qui officia deserunt mollitia animi, id est laborum et dolorum fuga. Et harum quidem rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio cumque nihil impedit quo minus.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="3" [disabled]="true">
                    <p-accordion-header>Header IV</p-accordion-header>
                </p-accordion-panel>
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule]
})
export class AccordionDisabledDemo {}
```

## Controlled

Panels can be controlled programmatically using value property as a model.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';
import { ButtonModule } from '@openng/optimus-ui/button';

@Component({
    template: `
        <div class="card">
            <div class="flex mb-4 gap-2 justify-end">
                <p-button (onClick)="active = '0'" [rounded]="true" label="1" styleClass="w-8 h-8 p-0" [outlined]="active !== '0'" />
                <p-button (onClick)="active = '1'" [rounded]="true" label="2" styleClass="w-8 h-8 p-0" [outlined]="active !== '1'" />
                <p-button (onClick)="active = '2'" [rounded]="true" label="3" styleClass="w-8 h-8 p-0" [outlined]="active !== '2'" />
            </div>
            <p-accordion [(value)]="active">
                <p-accordion-panel value="0">
                    <p-accordion-header>Header I</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
                            consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est
                            laborum.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="1">
                    <p-accordion-header>Header II</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="2">
                    <p-accordion-header>Header III</p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint occaecati cupiditate non provident, similique sunt in
                            culpa qui officia deserunt mollitia animi, id est laborum et dolorum fuga. Et harum quidem rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio cumque nihil impedit quo minus.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule, ButtonModule]
})
export class AccordionControlledDemo {
    active: string = '0';
}
```

## Template

Accordion is customized with toggleicon template.

```typescript
import { Component } from '@angular/core';
import { AccordionModule } from '@openng/optimus-ui/accordion';
import { AvatarModule } from '@openng/optimus-ui/avatar';
import { BadgeModule } from '@openng/optimus-ui/badge';

@Component({
    template: `
        <div class="card flex justify-content-center">
            <p-accordion value="0">
                <p-accordion-panel value="0">
                    <p-accordion-header>
                        <ng-template #toggleicon let-active="active">
                            @if (active) {
                                <i class="pi pi-minus"></i>
                            } @else {
                                <i class="pi pi-plus"></i>
                            }
                        </ng-template>
                        <span class="flex items-center gap-2 w-full">
                            <p-avatar image="https://optimus.openng.org/demo/avatar/amyelsner.png" shape="circle" />
                            <span class="font-bold whitespace-nowrap">Amy Elsner</span>
                            <p-badge value="3" class="ml-auto mr-2" />
                        </span>
                    </p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo
                            consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est
                            laborum.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="1">
                    <p-accordion-header>
                        <ng-template #toggleicon let-active="active">
                            @if (active) {
                                <i class="pi pi-minus"></i>
                            } @else {
                                <i class="pi pi-plus"></i>
                            }
                        </ng-template>
                        <span class="flex items-center gap-2 w-full">
                            <p-avatar image="https://optimus.openng.org/demo/avatar/onyamalimba.png" shape="circle" />
                            <span class="font-bold whitespace-nowrap">Onyama Limba</span>
                            <p-badge value="4" class="ml-auto mr-2" />
                        </span>
                    </p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            Sed ut perspiciatis unde omnis iste natus error sit voluptatem accusantium doloremque laudantium, totam rem aperiam, eaque ipsa quae ab illo inventore veritatis et quasi architecto beatae vitae dicta sunt explicabo. Nemo
                            enim ipsam voluptatem quia voluptas sit aspernatur aut odit aut fugit, sed quia consequuntur magni dolores eos qui ratione voluptatem sequi nesciunt. Consectetur, adipisci velit, sed quia non numquam eius modi.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
                <p-accordion-panel value="2">
                    <p-accordion-header>
                        <ng-template #toggleicon let-active="active">
                            @if (active) {
                                <i class="pi pi-minus"></i>
                            } @else {
                                <i class="pi pi-plus"></i>
                            }
                        </ng-template>
                        <span class="flex items-center gap-2 w-full">
                            <p-avatar image="https://optimus.openng.org/demo/avatar/ionibowcher.png" shape="circle" />
                            <span class="font-bold whitespace-nowrap">Ioni Bowcher</span>
                            <p-badge value="2" class="ml-auto mr-2" />
                        </span>
                    </p-accordion-header>
                    <p-accordion-content>
                        <p class="m-0">
                            At vero eos et accusamus et iusto odio dignissimos ducimus qui blanditiis praesentium voluptatum deleniti atque corrupti quos dolores et quas molestias excepturi sint occaecati cupiditate non provident, similique sunt in
                            culpa qui officia deserunt mollitia animi, id est laborum et dolorum fuga. Et harum quidem rerum facilis est et expedita distinctio. Nam libero tempore, cum soluta nobis est eligendi optio cumque nihil impedit quo minus.
                        </p>
                    </p-accordion-content>
                </p-accordion-panel>
            </p-accordion>
        </div>
    `,
    standalone: true,
    imports: [AccordionModule, AvatarModule, BadgeModule]
})
export class AccordionTemplateDemo {}
```

## Accessibility

Screen Reader Accordion header elements have a button role and use aria-controls to define the id of the content section along with aria-expanded for the visibility state. The value to read a header element defaults to the value of the header property and can be customized by defining an aria-label or aria-labelledby property. Each header has a heading role, for which the level is customized by headerAriaLevel and has a default level of 2 as per W3C specifications. Disabled accordions headers use aria-disabled and are excluded from the keybord navigation. The content uses region role, defines an id that matches the aria-controls of the header and aria-labelledby referring to the id of the header. Header Keyboard Support

## API

### Accordion

Selector: `p-accordion` · Component, standalone

Accordion groups a collection of contents in tabs.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `collapseIcon` | `string \| undefined` | - | Icon of an expanded tab. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `expandIcon` | `string \| undefined` | - | Icon of a collapsed tab. |
| `motionOptions` | `MotionOptions \| undefined` | - | The motion options. |
| `multiple` | `any` | `false` | When enabled, multiple tabs can be activated at the same time. |
| `pt` | `AccordionPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `selectOnFocus` | `any` | `false` | When enabled, the focused tab is activated. |
| `styleClass` | `string \| undefined` | - | Class of the element. **Deprecated**: since v20.0.0, use `class` instead. |
| `transitionOptions` | `string` | `'400ms cubic-bezier(0.86, 0, 0.07, 1)'` | Transition options of the animation. **Deprecated**: since v21.0.0, use `motionOptions` instead. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `string \| number \| string[] \| number[] \| null \| undefined` | - | Value of the active tab. Two-way: `[(value)]`. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onClose` | `AccordionTabCloseEvent` | Callback to invoke when an active tab is collapsed by clicking on the header. |
| `onOpen` | `AccordionTabOpenEvent` | Callback to invoke when a tab gets expanded. |
| `valueChange` | `string \| number \| string[] \| number[] \| null \| undefined` | Value of the active tab. |

### AccordionPanel

Selector: `p-accordion-panel, p-accordionpanel` · Component, standalone

AccordionPanel is a helper component for Accordion component.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `disabled` | `boolean` | `false` | Disables the tab when enabled. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pt` | `AccordionPanelPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `value` | `string \| number \| string[] \| number[] \| null \| undefined` | - | Value of the active tab. Two-way: `[(value)]`. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `valueChange` | `string \| number \| string[] \| number[] \| null \| undefined` | Value of the active tab. |

### AccordionHeader

Selector: `p-accordion-header, p-accordionheader` · Component, standalone

AccordionHeader is a helper component for Accordion component.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pt` | `AccordionHeaderPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-accordion-header` element.

| Name | Context | Description |
|---|---|---|
| `toggleicon` | `AccordionToggleIconTemplateContext` | Toggle icon template. |

### AccordionContent

Selector: `p-accordion-content, p-accordioncontent` · Component, standalone

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pt` | `AccordionContentPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

## Pass Through Options

### Accordion

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the root's DOM element. |
| `motion` | `MotionOptions` | Used to pass options to the motion component/directive. |

### AccordionPanel

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the panel's DOM element. |

### AccordionHeader

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the header's DOM element. |
| `toggleicon` | `PassThroughOption<SVGElement, I>` | Used to pass attributes to the toggle icon's DOM element. |

### AccordionContent

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the content container's DOM element. |
| `contentWrapper` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content wrapper DOM element. |
| `content` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the content's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-accordion` | Class name of the root element |
| `p-accordioncontent` | Class name of the content wrapper |
| `p-accordioncontent-content` | Class name of the content |
| `p-accordionheader` | Class name of the header |
| `p-accordionheader-toggle-icon` | Class name of the toggle icon |
| `p-accordionpanel` | Class name of the panel |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `accordion.transition.duration` | `--p-accordion-transition-duration` | Transition duration of root |
| `accordion.panel.border.width` | `--p-accordion-panel-border-width` | Border width of panel |
| `accordion.panel.border.color` | `--p-accordion-panel-border-color` | Border color of panel |
| `accordion.header.color` | `--p-accordion-header-color` | Color of header |
| `accordion.header.hover.color` | `--p-accordion-header-hover-color` | Hover color of header |
| `accordion.header.active.color` | `--p-accordion-header-active-color` | Active color of header |
| `accordion.header.active.hover.color` | `--p-accordion-header-active-hover-color` | Active hover color of header |
| `accordion.header.padding` | `--p-accordion-header-padding` | Padding of header |
| `accordion.header.font.weight` | `--p-accordion-header-font-weight` | Font weight of header |
| `accordion.header.border.radius` | `--p-accordion-header-border-radius` | Border radius of header |
| `accordion.header.border.width` | `--p-accordion-header-border-width` | Border width of header |
| `accordion.header.border.color` | `--p-accordion-header-border-color` | Border color of header |
| `accordion.header.background` | `--p-accordion-header-background` | Background of header |
| `accordion.header.hover.background` | `--p-accordion-header-hover-background` | Hover background of header |
| `accordion.header.active.background` | `--p-accordion-header-active-background` | Active background of header |
| `accordion.header.active.hover.background` | `--p-accordion-header-active-hover-background` | Active hover background of header |
| `accordion.header.focus.ring.width` | `--p-accordion-header-focus-ring-width` | Focus ring width of header |
| `accordion.header.focus.ring.style` | `--p-accordion-header-focus-ring-style` | Focus ring style of header |
| `accordion.header.focus.ring.color` | `--p-accordion-header-focus-ring-color` | Focus ring color of header |
| `accordion.header.focus.ring.offset` | `--p-accordion-header-focus-ring-offset` | Focus ring offset of header |
| `accordion.header.focus.ring.shadow` | `--p-accordion-header-focus-ring-shadow` | Focus ring shadow of header |
| `accordion.header.toggle.icon.color` | `--p-accordion-header-toggle-icon-color` | Toggle icon color of header |
| `accordion.header.toggle.icon.hover.color` | `--p-accordion-header-toggle-icon-hover-color` | Toggle icon hover color of header |
| `accordion.header.toggle.icon.active.color` | `--p-accordion-header-toggle-icon-active-color` | Toggle icon active color of header |
| `accordion.header.toggle.icon.active.hover.color` | `--p-accordion-header-toggle-icon-active-hover-color` | Toggle icon active hover color of header |
| `accordion.header.first.top.border.radius` | `--p-accordion-header-first-top-border-radius` | First top border radius of header |
| `accordion.header.first.border.width` | `--p-accordion-header-first-border-width` | First border width of header |
| `accordion.header.last.bottom.border.radius` | `--p-accordion-header-last-bottom-border-radius` | Last bottom border radius of header |
| `accordion.header.last.active.bottom.border.radius` | `--p-accordion-header-last-active-bottom-border-radius` | Last active bottom border radius of header |
| `accordion.content.border.width` | `--p-accordion-content-border-width` | Border width of content |
| `accordion.content.border.color` | `--p-accordion-content-border-color` | Border color of content |
| `accordion.content.background` | `--p-accordion-content-background` | Background of content |
| `accordion.content.color` | `--p-accordion-content-color` | Color of content |
| `accordion.content.padding` | `--p-accordion-content-padding` | Padding of content |
