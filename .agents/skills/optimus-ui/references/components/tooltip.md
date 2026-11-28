# Tooltip

Tooltip directive provides advisory information for a component. Tooltip is integrated within various Optimus UI components.

## Import

```typescript
import { TooltipModule, Tooltip } from '@openng/optimus-ui/tooltip';
import { TooltipOptions } from '@openng/optimus-ui/api';
```

## Position

Position of the tooltip is specified using tooltipPosition attribute. Valid values are top , bottom , right and left . Default position of the tooltip is right .

```typescript
import { Component } from '@angular/core';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex flex-wrap justify-center gap-2">
            <input type="text" pInputText pTooltip="Enter your username" tooltipPosition="right" placeholder="Right" />
            <input type="text" pInputText pTooltip="Enter your username" tooltipPosition="top" placeholder="Top" />
            <input type="text" pInputText pTooltip="Enter your username" tooltipPosition="bottom" placeholder="Bottom" />
            <input type="text" pInputText pTooltip="Enter your username" tooltipPosition="left" placeholder="Left" />
        </div>
    `,
    standalone: true,
    imports: [InputTextModule, TooltipModule]
})
export class TooltipPositionDemo {}
```

## Event

Tooltip gets displayed on hover event of its target by default, other option is the focus event to display and blur to hide.

```typescript
import { Component } from '@angular/core';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex flex-wrap justify-center gap-2">
            <input type="text" pInputText pTooltip="Enter your username" tooltipEvent="focus" placeholder="focus to display tooltip" />
        </div>
    `,
    standalone: true,
    imports: [InputTextModule, TooltipModule]
})
export class TooltipEventDemo {}
```

## Auto Hide

Tooltip is hidden when mouse leaves the target element, in cases where tooltip needs to be interacted with, set autoHide to false to change the default behavior.

```typescript
import { Component } from '@angular/core';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex flex-wrap justify-center gap-2">
            <input type="text" pInputText pTooltip="Enter your username" [autoHide]="false" placeholder="autoHide: false" />
            <input type="text" pInputText pTooltip="Enter your username" placeholder="autoHide: true" />
        </div>
    `,
    standalone: true,
    imports: [InputTextModule, TooltipModule]
})
export class TooltipAutohideDemo {}
```

## Delay

Adding delays to the show and hide events are defined with showDelay and hideDelay options respectively.

```typescript
import { Component } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-button pTooltip="Confirm to proceed" showDelay="1000" hideDelay="300" label="Save" />
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, TooltipModule]
})
export class TooltipDelayDemo {}
```

## Custom

Tooltip can use either a string or a TemplateRef .

```typescript
import { Component } from '@angular/core';
import { ButtonModule } from '@openng/optimus-ui/button';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-button [pTooltip]="tooltipContent" severity="secondary" tooltipPosition="bottom" label="Button" />
            <ng-template #tooltipContent>
                <div class="flex items-center">
                    <img src="https://optimus.openng.org/logo-icon.svg" alt="Optimus UI" width="32" height="32" class="mr-2" />
                    <span> <b>Optimus UI</b> rocks! </span>
                </div>
            </ng-template>
        </div>
    `,
    standalone: true,
    imports: [ButtonModule, TooltipModule]
})
export class TooltipCustomDemo {}
```

## Tooltip Options

Tooltip is also configurable by using tooltipOptions property.

```typescript
import { Component } from '@angular/core';
import { InputTextModule } from '@openng/optimus-ui/inputtext';
import { TooltipModule } from '@openng/optimus-ui/tooltip';

@Component({
    template: `
        <div class="card flex justify-center">
            <input type="text" pInputText pTooltip="Enter your username" [tooltipOptions]="tooltipOptions" placeholder="hover to display tooltip" />
        </div>
    `,
    standalone: true,
    imports: [InputTextModule, TooltipModule]
})
export class TooltipOptionsDemo {
    tooltipOptions: any;
}
```

## Accessibility

Screen Reader Tooltip component uses tooltip role and when it becomes visible the generated id of the tooltip is defined as the aria-describedby of the target. Keyboard Support Key Function escape Closes the tooltip when focus is on the target.

## API

### Tooltip

Selector: `[pTooltip]` · Directive, standalone

Tooltip directive provides advisory information for a component.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `appendTo` | `any` | - | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `autoHide` | `boolean` | `true` | Whether to hide tooltip when hovering over tooltip content. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `escape` | `boolean` | `true` | By default the tooltip contents are rendered as text. Set to false to support html tags in the content. |
| `fitContent` | `boolean` | `true` | Automatically adjusts the element position when there is not enough space on the selected position. |
| `hideDelay` | `number \| undefined` | - | Delay to hide the tooltip in milliseconds. |
| `hideOnEscape` | `boolean` | `true` | Whether to hide tooltip on escape key press. |
| `life` | `number \| undefined` | - | Time to wait in milliseconds to hide the tooltip even it is active. |
| `positionLeft` | `number \| undefined` | - | Specifies the additional horizontal offset of the tooltip from its default position. |
| `positionStyle` | `string \| undefined` | - | Type of CSS position. |
| `positionTop` | `number \| undefined` | - | Specifies the additional vertical offset of the tooltip from its default position. |
| `pt` | `TooltipPassThroughOptions<unknown> \| undefined` | - | Used to pass attributes to DOM elements inside the component. |
| `pTooltip` | `string \| TemplateRef<HTMLElement> \| undefined` | - | Content of the tooltip. |
| `pTooltipPT` | `TooltipPassThrough` | - | Used to pass attributes to DOM elements inside the Tooltip component. |
| `pTooltipUnstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `ptTooltip` | `TooltipPassThrough` | - | Used to pass attributes to DOM elements inside the Tooltip component. **Deprecated**: use pTooltipPT instead. |
| `showDelay` | `number \| undefined` | - | Delay to show the tooltip in milliseconds. |
| `showOnEllipsis` | `boolean` | `false` | Whether to show the tooltip only when the target text overflows (e.g., ellipsis is active). |
| `tooltipDisabled` | `boolean` | `false` | When present, it specifies that the component should be disabled. |
| `tooltipEvent` | `"both" \| "hover" \| "focus"` | `'hover'` | Event to show the tooltip. |
| `tooltipOptions` | `TooltipOptions \| undefined` | - | Specifies the tooltip configuration options for the component. |
| `tooltipPosition` | `string \| undefined` | - | Position of the tooltip. |
| `tooltipStyleClass` | `string \| undefined` | - | Style class of the tooltip. |
| `tooltipZIndex` | `string \| undefined` | - | Whether the z-index should be managed automatically to always go on top or have a fixed value. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

### TooltipOptions

Defines options of Tooltip.

| Name | Type | Description |
|---|---|---|
| `tooltipLabel?` | `string \| undefined` | Label of tooltip. |
| `tooltipPosition?` | `"top" \| "bottom" \| "left" \| "right" \| undefined` | Position of tooltip. |
| `tooltipEvent?` | `"both" \| "hover" \| "focus" \| undefined` | Event to show the tooltip. |
| `appendTo?` | `any` | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `positionStyle?` | `string \| undefined` | Type of CSS position. |
| `tooltipStyleClass?` | `string \| undefined` | Style class of the tooltip. |
| `tooltipZIndex?` | `string \| undefined` | Whether the z-index should be managed automatically to always go on top or have a fixed value. |
| `escape?` | `boolean \| undefined` | By default the tooltip contents are rendered as text. Set to false to support html tags in the content. |
| `disabled?` | `boolean \| undefined` | When present, it specifies that the component should be disabled. |
| `positionTop?` | `number \| undefined` | Specifies the additional vertical offset of the tooltip from its default position. |
| `positionLeft?` | `number \| undefined` | Specifies the additional horizontal offset of the tooltip from its default position. |
| `showDelay?` | `number \| undefined` | Delay to show the tooltip in milliseconds. |
| `hideDelay?` | `number \| undefined` | Delay to hide the tooltip in milliseconds. |
| `life?` | `number \| undefined` | Time to wait in milliseconds to hide the tooltip even it is active. |
| `id?` | `string \| undefined` | When present, it adds a custom id to the tooltip. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `arrow` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the arrow's DOM element. |
| `text` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the text's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-tooltip` | Class name of the root element |
| `p-tooltip-arrow` | Class name of the arrow element |
| `p-tooltip-text` | Class name of the text element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `tooltip.max.width` | `--p-tooltip-max-width` | Max width of root |
| `tooltip.gutter` | `--p-tooltip-gutter` | Gutter of root |
| `tooltip.shadow` | `--p-tooltip-shadow` | Shadow of root |
| `tooltip.padding` | `--p-tooltip-padding` | Padding of root |
| `tooltip.border.radius` | `--p-tooltip-border-radius` | Border radius of root |
| `tooltip.background` | `--p-tooltip-background` | Background of root |
| `tooltip.color` | `--p-tooltip-color` | Color of root |
