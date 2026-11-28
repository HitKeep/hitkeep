# SplitButton

SplitButton groups a set of commands in an overlay with a default action item.

## Import

```typescript
import { SplitButtonModule, SplitButton } from '@openng/optimus-ui/splitbutton';
```

## Basic

SplitButton has a default action button and a collection of additional options defined by the model property based on MenuModel API.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonBasicDemo {
    private messageService = inject(MessageService);

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Updated', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'warn', summary: 'Delete', detail: 'Data Deleted' });
    }
}
```

## Icons

The buttons and menuitems have support to display icons.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-splitbutton label="Save" icon="pi pi-check" dropdownIcon="pi pi-cog" [model]="items" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonIconsDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        icon: 'pi pi-refresh',
                        command: () => {
                            this.messageService.add({ severity: 'success', summary: 'Updated', detail: 'Data Updated', life: 3000 });
                        }
                    },
                    {
                        label: 'Delete',
                        icon: 'pi pi-times',
                        command: () => {
                            this.messageService.add({ severity: 'warn', summary: 'Delete', detail: 'Data Deleted', life: 3000 });
                        }
                    },
                    {
                        separator: true
                    },
                    {
                        label: 'Quit',
                        icon: 'pi pi-power-off',
                        command: () => {
                            window.open('https://angular.io/', '_blank');
                        }
                    }
                ];
    }
}
```

## Nested

SplitButton has a default action button and a collection of additional options defined by the model property based on MenuModel API.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-splitbutton label="Save" (onClick)="save('info')" [model]="items" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonNestedDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'File',
                        icon: 'pi pi-fw pi-file',
                        items: [
                            {
                                label: 'New',
                                icon: 'pi pi-fw pi-plus',
                                items: [
                                    {
                                        label: 'Bookmark',
                                        icon: 'pi pi-fw pi-bookmark'
                                    },
                                    {
                                        label: 'Video',
                                        icon: 'pi pi-fw pi-video'
                                    }
                                ]
                            },
                            {
                                label: 'Delete',
                                icon: 'pi pi-fw pi-trash'
                            },
                            {
                                separator: true
                            },
                            {
                                label: 'Export',
                                icon: 'pi pi-fw pi-external-link'
                            }
                        ]
                    },
                    {
                        label: 'Edit',
                        icon: 'pi pi-fw pi-pencil',
                        items: [
                            {
                                label: 'Left',
                                icon: 'pi pi-fw pi-align-left'
                            },
                            {
                                label: 'Right',
                                icon: 'pi pi-fw pi-align-right'
                            },
                            {
                                label: 'Center',
                                icon: 'pi pi-fw pi-align-center'
                            },
                            {
                                label: 'Justify',
                                icon: 'pi pi-fw pi-align-justify'
                            }
                        ]
                    },
                    {
                        label: 'Users',
                        icon: 'pi pi-fw pi-user',
                        items: [
                            {
                                label: 'New',
                                icon: 'pi pi-fw pi-user-plus'
                            },
                            {
                                label: 'Delete',
                                icon: 'pi pi-fw pi-user-minus'
                            },
                            {
                                label: 'Search',
                                icon: 'pi pi-fw pi-users',
                                items: [
                                    {
                                        label: 'Filter',
                                        icon: 'pi pi-fw pi-filter',
                                        items: [
                                            {
                                                label: 'Print',
                                                icon: 'pi pi-fw pi-print'
                                            }
                                        ]
                                    },
                                    {
                                        icon: 'pi pi-fw pi-bars',
                                        label: 'List'
                                    }
                                ]
                            }
                        ]
                    },
                    {
                        label: 'Events',
                        icon: 'pi pi-fw pi-calendar',
                        items: [
                            {
                                label: 'Edit',
                                icon: 'pi pi-fw pi-pencil',
                                items: [
                                    {
                                        label: 'Save',
                                        icon: 'pi pi-fw pi-calendar-plus'
                                    },
                                    {
                                        label: 'Delete',
                                        icon: 'pi pi-fw pi-calendar-minus'
                                    }
                                ]
                            },
                            {
                                label: 'Archieve',
                                icon: 'pi pi-fw pi-calendar-times',
                                items: [
                                    {
                                        label: 'Remove',
                                        icon: 'pi pi-fw pi-calendar-minus'
                                    }
                                ]
                            }
                        ]
                    },
                    {
                        separator: true
                    },
                    {
                        label: 'Quit',
                        icon: 'pi pi-fw pi-power-off'
                    }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }
}
```

## Severity

The severity property defines the type of button.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="secondary" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="success" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="info" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="warn" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="help" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="danger" />
            <p-splitbutton label="Save" (onClick)="save()" [model]="items" severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonSeverityDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Updated', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'warn', summary: 'Delete', detail: 'Data Deleted' });
    }
}
```

## Disabled

When the disabled attribute is present, the element is uneditable and unfocused. Additionally, the disabled states of the button and menu button can be handled independently. The button is disabled when buttonDisabled is present, and the menu button is disabled when menuButtonDisabled is present.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-splitbutton label="Save" icon="pi pi-plus" (onClick)="save('info')" [model]="items" [disabled]="true" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonDisabledDemo {
    private messageService = inject(MessageService);

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Raised

Raised buttons display a shadow to indicate elevation.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Primary" [model]="items" (onClick)="save('info')" raised />
            <p-splitbutton label="Secondary" [model]="items" (onClick)="save('info')" raised severity="secondary" />
            <p-splitbutton label="Success" [model]="items" (onClick)="save('info')" raised severity="success" />
            <p-splitbutton label="Info" [model]="items" (onClick)="save('info')" raised severity="info" />
            <p-splitbutton label="Warning" [model]="items" (onClick)="save('info')" raised severity="warn" />
            <p-splitbutton label="Help" [model]="items" (onClick)="save('info')" raised severity="help" />
            <p-splitbutton label="Danger" [model]="items" (onClick)="save('info')" raised severity="danger" />
            <p-splitbutton label="Contrast" (onClick)="save('info')" [model]="items" severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonRaisedDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Rounded

Rounded buttons have a circular border radius.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Primary" [model]="items" (onClick)="save('info')" rounded />
            <p-splitbutton label="Secondary" [model]="items" (onClick)="save('info')" rounded severity="secondary" />
            <p-splitbutton label="Success" [model]="items" (onClick)="save('info')" rounded severity="success" />
            <p-splitbutton label="Info" [model]="items" (onClick)="save('info')" rounded severity="info" />
            <p-splitbutton label="Warning" [model]="items" (onClick)="save('info')" rounded severity="warn" />
            <p-splitbutton label="Help" [model]="items" (onClick)="save('info')" rounded severity="help" />
            <p-splitbutton label="Danger" [model]="items" (onClick)="save('info')" rounded severity="danger" />
            <p-splitbutton label="Contrast" [model]="items" (onClick)="save('info')" rounded severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonRoundedDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Text

Text buttons are displayed as textual elements.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Primary" [model]="items" (onClick)="save('info')" text />
            <p-splitbutton label="Secondary" [model]="items" (onClick)="save('info')" text severity="secondary" />
            <p-splitbutton label="Success" [model]="items" (onClick)="save('info')" text severity="success" />
            <p-splitbutton label="Info" [model]="items" (onClick)="save('info')" text severity="info" />
            <p-splitbutton label="Warning" [model]="items" (onClick)="save('info')" text severity="warn" />
            <p-splitbutton label="Help" [model]="items" (onClick)="save('info')" text severity="help" />
            <p-splitbutton label="Danger" [model]="items" (onClick)="save('info')" text severity="danger" />
            <p-splitbutton label="Contrast" [model]="items" (onClick)="save('info')" text severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonTextDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Raised Text

Text buttons can be displayed as raised as well for elevation.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Primary" [model]="items" (onClick)="save('info')" raised text />
            <p-splitbutton label="Secondary" [model]="items" (onClick)="save('info')" raised text severity="secondary" />
            <p-splitbutton label="Success" [model]="items" (onClick)="save('info')" raised text severity="success" />
            <p-splitbutton label="Info" [model]="items" (onClick)="save('info')" raised text severity="info" />
            <p-splitbutton label="Warning" [model]="items" (onClick)="save('info')" raised text severity="warn" />
            <p-splitbutton label="Help" [model]="items" (onClick)="save('info')" raised text severity="help" />
            <p-splitbutton label="Danger" [model]="items" (onClick)="save('info')" raised text severity="danger" />
            <p-splitbutton label="Contrast" [model]="items" (onClick)="save('info')" raised text severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonRaisedtextDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Outlined

Outlined buttons display a border without a background initially.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center flex-wrap gap-4">
            <p-toast />
            <p-splitbutton label="Primary" [model]="items" (onClick)="save('info')" outlined />
            <p-splitbutton label="Secondary" [model]="items" (onClick)="save('info')" outlined severity="secondary" />
            <p-splitbutton label="Success" [model]="items" (onClick)="save('info')" outlined severity="success" />
            <p-splitbutton label="Info" [model]="items" (onClick)="save('info')" outlined severity="info" />
            <p-splitbutton label="Warning" [model]="items" (onClick)="save('info')" outlined severity="warn" />
            <p-splitbutton label="Help" [model]="items" (onClick)="save('info')" outlined severity="help" />
            <p-splitbutton label="Danger" [model]="items" (onClick)="save('info')" outlined severity="danger" />
            <p-splitbutton label="Contrast" [model]="items" (onClick)="save('info')" outlined severity="contrast" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonOutlinedDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Sizes

SplitButton provides small and large sizes as alternatives to the standard.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex flex-wrap justify-center items-center gap-4">
            <p-toast />
            <p-splitbutton label="Small" [model]="items" (onClick)="save('info')" size="small" />
            <p-splitbutton label="Normal" [model]="items" (onClick)="save('info')" />
            <p-splitbutton label="Large" [model]="items" (onClick)="save('info')" size="large" />
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonSizesDemo {
    private messageService = inject(MessageService);

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save(severity: string) {
        this.messageService.add({ severity: severity, summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Deleted' });
    }
}
```

## Template

SplitButton has a default action button and a collection of additional options defined by the model property based on MenuModel API.

```typescript
import { Component, inject } from '@angular/core';
import { SplitButtonModule } from '@openng/optimus-ui/splitbutton';
import { ToastModule } from '@openng/optimus-ui/toast';
import { MenuItem, MessageService } from '@openng/optimus-ui/api';

@Component({
    template: `
        <div class="card flex justify-center">
            <p-toast />
            <p-splitbutton label="Save" (onClick)="save()" severity="contrast" [model]="items">
                <ng-template #content>
                    <span class="flex items-center font-bold">
                        <img alt="logo" src="logo-icon.svg" style="height: 1rem; margin-right: 0.5rem" />
                        <span>Optimus UI</span>
                    </span>
                </ng-template>
            </p-splitbutton>
        </div>
    `,
    standalone: true,
    imports: [SplitButtonModule, ToastModule],
    providers: [MessageService]
})
export class SplitbuttonTemplateDemo {
    private messageService = inject(MessageService);
    items: MenuItem[];

    constructor() {
        this.items = [
                    {
                        label: 'Update',
                        command: () => {
                            this.update();
                        }
                    },
                    {
                        label: 'Delete',
                        command: () => {
                            this.delete();
                        }
                    },
                    { label: 'Angular.dev', url: 'https://angular.dev' },
                    { separator: true },
                    { label: 'Upload', routerLink: ['/fileupload'] }
                ];
    }

    save() {
        this.messageService.add({ severity: 'success', summary: 'Success', detail: 'Data Saved' });
    }

    update() {
        this.messageService.add({ severity: 'success', summary: 'Updated', detail: 'Data Updated' });
    }

    delete() {
        this.messageService.add({ severity: 'warn', summary: 'Delete', detail: 'Data Deleted' });
    }
}
```

## Accessibility

Screen Reader SplitButton component renders two native button elements, main button uses the label property to define aria-label by default which can be customized with buttonProps . Dropdown button requires an explicit definition to describe it using menuButtonProps option and also includes aria-haspopup , aria-expanded for states along with aria-controls to define the relation between the popup and the button. The popup overlay uses menu role on the list and each action item has a menuitem role with an aria-label as the menuitem label. The id of the menu refers to the aria-controls of the dropdown button.

## Reversedkeys

Following keys are reserved in the preset scheme and cannot be used as a token name; primitive , semantic , components , directives , colorscheme , light , dark , common , root , states and extend .

## API

### SplitButton

Selector: `p-splitbutton, p-splitButton, p-split-button` · Component, standalone

SplitButton groups a set of commands in an overlay with a default command.

#### Inputs

| Name | Type | Default | Description |
|---|---|---|---|
| `appendTo` | `any` | `'body'` | Target element to attach the overlay, valid values are "body" or a local ng-template variable of another element (note: use binding with brackets for template variables, e.g. [appendTo]="mydiv" for a div element having #mydiv as variable name). |
| `autofocus` | `boolean \| undefined` | - | When present, it specifies that the component should automatically get focus on load. |
| `buttonDisabled` | `boolean` | `false` | When present, it specifies that the button element should be disabled. |
| `buttonProps` | `ButtonProps \| undefined` | - | Button Props |
| `dir` | `string \| undefined` | - | Indicates the direction of the element. |
| `disabled` | `boolean \| undefined` | - | When present, it specifies that the element should be disabled. |
| `dropdownIcon` | `string \| undefined` | - | Name of the dropdown icon. |
| `dt` | `Object \| undefined` | - | Defines scoped design tokens of the component. |
| `expandAriaLabel` | `string \| undefined` | - | Defines a string that labels the expand button for accessibility. |
| `hideTransitionOptions` | `string` | `'.1s linear'` | Transition options of the hide animation. **Deprecated**: since v21.0.0. Use `motionOptions` instead. |
| `icon` | `string \| undefined` | - | Name of the icon. |
| `iconPos` | `"left" \| "right"` | `'left'` | Position of the icon. |
| `label` | `string \| undefined` | - | Text of the button. |
| `menuButtonDisabled` | `boolean` | `false` | When present, it specifies that the menu button element should be disabled. |
| `menuButtonProps` | `MenuButtonProps \| undefined` | - | Menu Button Props |
| `menuStyle` | `{ [klass: string]: any; } \| null \| undefined` | - | Inline style of the overlay menu. |
| `menuStyleClass` | `string \| undefined` | - | Style class of the overlay menu. |
| `model` | `MenuItem[] \| undefined` | - | MenuModel instance to define the overlay items. |
| `motionOptions` | `MotionOptions \| undefined` | - | The motion options. |
| `outlined` | `boolean` | `false` | Add a border class without a background initially. |
| `plain` | `boolean` | `false` | Add a plain textual class to the button without a background initially. |
| `pt` | `SplitButtonPassThrough` | - | Used to pass attributes to DOM elements inside the component. |
| `ptOptions` | `PassThroughOptions \| undefined` | - | Used to configure passthrough(pt) options of the component. |
| `raised` | `boolean` | `false` | Add a shadow to indicate elevation. |
| `rounded` | `boolean` | `false` | Add a circular border radius to the button. |
| `severity` | `"success" \| "info" \| "warn" \| "danger" \| "secondary" \| "contrast" \| "help" \| "primary"` | - | Defines the style of the button. |
| `showTransitionOptions` | `string` | `'.12s cubic-bezier(0, 0, 0.2, 1)'` | Transition options of the show animation. **Deprecated**: since v21.0.0. Use `motionOptions` instead. |
| `size` | `"large" \| "small"` | - | Defines the size of the button. |
| `styleClass` | `string \| undefined` | - | Class of the element. **Deprecated**: since v20.0.0, use `class` instead. |
| `tabindex` | `number \| undefined` | - | Index of the element in tabbing order. |
| `text` | `boolean` | `false` | Add a textual class to the button without a background initially. |
| `tooltip` | `string \| undefined` | - | Tooltip for the main button. |
| `tooltipOptions` | `TooltipOptions \| undefined` | - | Tooltip options for the main button. |
| `unstyled` | `boolean \| undefined` | - | Indicates whether the component should be rendered without styles. |

#### Outputs

| Name | Payload | Description |
|---|---|---|
| `onClick` | `MouseEvent` | Callback to invoke when default command button is clicked. |
| `onDropdownClick` | `MouseEvent` | Callback to invoke when dropdown button is clicked. |
| `onMenuHide` | `any` | Callback to invoke when overlay menu is hidden. |
| `onMenuShow` | `any` | Callback to invoke when overlay menu is shown. |

#### Templates

Define with `<ng-template #name let-context>` inside the `p-splitbutton` element.

| Name | Context | Description |
|---|---|---|
| `content` | - | Custom content template. |
| `dropdownicon` | - | Custom dropdown icon template. |

## Pass Through Options

| Name | Type | Description |
|---|---|---|
| `root` | `PassThroughOption<HTMLDivElement, I>` | Used to pass attributes to the root's DOM element. |
| `pcButton` | `ButtonPassThrough` | Used to pass attributes to the Button component. |
| `pcDropdown` | `ButtonPassThrough` | Used to pass attributes to the dropdown Button component. |
| `pcMenu` | `MenuPassThrough` | Used to pass attributes to the TieredMenu component. |

## Theming

### CSS Classes

| Class | Description |
|---|---|
| `p-splitbutton` | Class name of the root element |
| `p-splitbutton-button` | Class name of the button element |
| `p-splitbutton-dropdown` | Class name of the dropdown element |

### Design Tokens

| Token | CSS Variable | Description |
|---|---|---|
| `splitbutton.border.radius` | `--p-splitbutton-border-radius` | Border radius of root |
| `splitbutton.rounded.border.radius` | `--p-splitbutton-rounded-border-radius` | Rounded border radius of root |
| `splitbutton.raised.shadow` | `--p-splitbutton-raised-shadow` | Raised shadow of root |
