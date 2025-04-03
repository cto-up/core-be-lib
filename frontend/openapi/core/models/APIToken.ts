/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

import type { NewAPIToken } from './NewAPIToken';

export type APIToken = (NewAPIToken & {
    id: string;
    status: APIToken.status;
    /**
     * Whether this token has been revoked
     */
    revoked: boolean;
    /**
     * Token revoked at timestamp
     */
    revokedAt?: string;
    /**
     * Revoked reason
     */
    revokedReason?: string;
    /**
     * User of the token revoker
     */
    revokedBy?: string;
    /**
     * Last used timestamp
     */
    lastUsedAt?: string;
    /**
     * Last used IP address
     */
    lastUsedIp?: string;
    /**
     * User ID of the token creator
     */
    createdBy: string;
    /**
     * Token creation timestamp
     */
    createdAt: string;
    /**
     * Token last update timestamp
     */
    updatedAt: string;
});

export namespace APIToken {

    export enum status {
        ACTIVE = 'ACTIVE',
        REVOKED = 'REVOKED',
        EXPIRED = 'EXPIRED',
    }


}

