/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

export type APITokenAuditLog = {
    id: string;
    tokenId: string;
    action: APITokenAuditLog.action;
    ipAddress?: string | null;
    userAgent?: string | null;
    timestamp: string;
    additionalData?: Record<string, any> | null;
};

export namespace APITokenAuditLog {

    export enum action {
        CREATED = 'CREATED',
        USED = 'USED',
        REVOKED = 'REVOKED',
        UPDATED = 'UPDATED',
    }


}

