/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

import type { NewClientApplication } from './NewClientApplication';

export type ClientApplication = (NewClientApplication & {
    id: string;
    lastUsedAt?: string;
    /**
     * If null, this is a global application managed by SUPER_ADMIN
     */
    tenantId?: string | null;
});

