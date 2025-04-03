/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

import type { Role } from './Role';
import type { UserProfileSchema } from './UserProfileSchema';

export type User = {
    id: string;
    name: string;
    email: string;
    disabled?: boolean;
    email_verified?: boolean;
    profile?: UserProfileSchema;
    roles?: Array<Role>;
    created_at?: string;
};

