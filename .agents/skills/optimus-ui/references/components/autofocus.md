# AutoFocus

AutoFocus manages focus on focusable element on load.

## Import

```typescript
import { AutoFocusModule, AutoFocus } from '@openng/optimus-ui/autofocus';
```

## Basic

AutoFocus is applied to any focusable input element with the pAutoFocus directive.

```typescript
import { Component } from '@angular/core';
import { InputTextModule } from '@openng/optimus-ui/inputtext';

@Component({
    template: `
        <div class="card flex justify-center">
            <input type="text" pInputText [pAutoFocus]="true" placeholder="Automatically focused" />
        </div>
    `,
    standalone: true,
    imports: [InputTextModule]
})
export class AutofocusBasicDemo {}
```

## API

### AutoFocus

Selector: `[pAutoFocus]` · Directive, standalone

AutoFocus manages focus on focusable element on load.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `pAutoFocus` | `boolean` | `false` | When present, it specifies that the component should automatically get focus on load. |
| `pt` | `any` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |
