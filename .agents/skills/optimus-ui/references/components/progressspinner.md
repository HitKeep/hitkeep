# ProgressSpinner

ProgressSpinner is a process status indicator.

## Import

```typescript
import { ProgressSpinnerModule, ProgressSpinner } from '@openng/optimus-ui/progressspinner';
```

## Basic

An infinite spin animation is displayed by default.

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-progress-spinner ariaLabel="loading" />
        </div>
    `,
    standalone: true,
    imports: []
})
export class ProgressspinnerBasicDemo {}
```

## Custom

ProgressSpinner can be customized with styling property like strokeWidth and fill .

```typescript
import { Component } from '@angular/core';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-progress-spinner strokeWidth="8" fill="transparent" animationDuration=".5s" [style]="{ width: '50px', height: '50px' }" />
        </div>
    `,
    standalone: true,
    imports: []
})
export class ProgressspinnerCustomDemo {}
```

## Accessibility

Screen Reader ProgressSpinner components uses progressbar role. Value to describe the component can be defined using aria-labelledby and aria-label props.

## API

### ProgressSpinner

Selector: `p-progressSpinner, p-progress-spinner, p-progressspinner` · Component, standalone

ProgressSpinner is a process status indicator.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `animationDuration` | `string` | `'2s'` | Duration of the rotate animation. |
| `ariaLabel` | `string \| undefined` | - | Used to define a aria label attribute the current element. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `fill` | `string` | `'none'` | Color for the background of the circle. |
| `pt` | `ProgressSpinnerPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `strokeWidth` | `string` | `'2'` | Width of the circle stroke. |
| `styleClass` | `string \| undefined` | - | Class of the element. **Deprecated**: since v20.0.0, use `class` instead. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `host` | `PassThroughOption<HTMLElement, I>` | Used to pass attributes to the host's DOM element. |
| `root` | `PassThroughOption<SVGSVGElement, I>` | Used to pass attributes to the root's DOM element. |
| `spin` | `PassThroughOption<SVGSVGElement, I>` | Used to pass attributes to the spin's DOM element. |
| `circle` | `PassThroughOption<SVGCircleElement, I>` | Used to pass attributes to the circle's DOM element. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-progressspinner` | Class name of the root element |
| `p-progressspinner-spin` | Class name of the spin element |
| `p-progressspinner-circle` | Class name of the circle element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `progressspinner.color.one` | `--p-progressspinner-color-one` | Color one of root |
| `progressspinner.color.two` | `--p-progressspinner-color-two` | Color two of root |
| `progressspinner.color.three` | `--p-progressspinner-color-three` | Color three of root |
| `progressspinner.color.four` | `--p-progressspinner-color-four` | Color four of root |
