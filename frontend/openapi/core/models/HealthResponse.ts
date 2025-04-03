/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

import type { CheckDetails } from './CheckDetails';

export type HealthResponse = {
    /**
     * The overall health status of the API
     */
    status: HealthResponse.status;
    /**
     * The version of the API
     */
    version: string;
    /**
     * An identifier for this deployment or release of the API
     */
    releaseId?: string;
    /**
     * Array of notes relevant to current state of health
     */
    notes?: Array<string>;
    /**
     * Raw error output, in case of "fail" or "warn" states
     */
    output?: string;
    /**
     * Detailed health check results for API components or dependencies
     */
    checks?: Record<string, CheckDetails>;
};

export namespace HealthResponse {

    /**
     * The overall health status of the API
     */
    export enum status {
        PASS = 'pass',
        FAIL = 'fail',
        WARN = 'warn',
    }


}

