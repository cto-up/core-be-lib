/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

export type NewAPIToken = {
    name: string;
    description?: string;
    expiresAt: string;
    /**
     * ID of the client application this token belongs to
     */
    clientApplicationId: string;
    /**
     * Name of the client application
     */
    applicationName: string;
    /**
     * First few characters of the token for identification
     */
    tokenPrefix: string;
    /**
     * Permission scopes for this token
     */
    scopes?: Array<string> | null;
};

