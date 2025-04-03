/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { HealthResponse } from '../models/HealthResponse';

import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';

export class HealthService {

    /**
     * API Health Check
     * Returns the health status of the API and its dependencies
     * @returns HealthResponse Healthy
     * @throws ApiError
     */
    public static getHealthCheck(): CancelablePromise<HealthResponse> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/public-api/v1/health',
            errors: {
                503: `Unhealthy`,
            },
        });
    }

}
