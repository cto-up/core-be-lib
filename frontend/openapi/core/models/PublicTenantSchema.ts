/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

import type { Tenant } from './Tenant';
import type { TenantFeatures } from './TenantFeatures';
import type { TenantProfileSchema } from './TenantProfileSchema';

export type PublicTenantSchema = (Tenant & {
    profile: TenantProfileSchema;
    features: TenantFeatures;
});

