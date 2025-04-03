/* generated using openapi-typescript-codegen -- do no edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */

export type CheckDetails = {
    /**
     * The type of component (e.g., database, cache, external service)
     */
    componentType: string;
    /**
     * The name of the component being checked
     */
    componentName?: string;
    /**
     * The status of this particular component
     */
    status: CheckDetails.status;
    /**
     * The time at which this check was performed
     */
    time?: string;
    /**
     * Any additional information or error output for this check
     */
    output?: string;
};

export namespace CheckDetails {

    /**
     * The status of this particular component
     */
    export enum status {
        PASS = 'pass',
        FAIL = 'fail',
        WARN = 'warn',
    }


}

