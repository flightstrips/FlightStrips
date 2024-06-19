/* eslint-disable */
/* tslint:disable */
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export interface AcceptCoordinationRequestModel {
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  frequency: string
}

export interface Bay {
  /** @minLength 1 */
  name: string
  default: BayDefaultType
  callsignFilter: string[]
}

export enum BayDefaultType {
  Arrival = 'Arrival',
  Departure = 'Departure',
  None = 'None',
}

export enum CommunicationType {
  Unassigned = 'Unassigned',
  Voice = 'Voice',
  Receive = 'Receive',
  Text = 'Text',
}

export interface CoordinationResponseModel {
  /** @format int32 */
  id: number
  state: CoordinationState
  /** @minLength 1 */
  callsign: string
  /** @minLength 1 */
  fromFrequency: string
  /** @minLength 1 */
  toFrequency: string
}

export enum CoordinationState {
  Transfer = 'Transfer',
}

export interface OnlinePositionResponseModel {
  position?: string | null
  frequency?: string | null
}

export interface ProblemDetails {
  type?: string | null
  title?: string | null
  /** @format int32 */
  status?: number | null
  detail?: string | null
  instance?: string | null
  [key: string]: any
}

export interface RejectCoordinationRequestModel {
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  frequency: string
}

export interface RunwayConfigResponseModel {
  /** @minLength 1 */
  departure: string
  /** @minLength 1 */
  arrival: string
  /** @minLength 1 */
  position: string
}

export interface SessionModel {
  name?: string | null
  airport?: string | null
}

export interface SessionResponseModel {
  sessions?: SessionModel[] | null
}

export interface StripAssumeRequestModel {
  /** @minLength 1 */
  frequency: string
  force: boolean
  /** @minLength 1 */
  position: string
}

export interface StripClearRequestModel {
  isCleared: boolean
  /** @minLength 1 */
  position: string
}

export interface StripMoveRequestModel {
  /** @minLength 1 */
  bay: string
  /** @format int32 */
  sequence?: number | null
  /** @minLength 1 */
  position: string
}

export interface StripResponseModel {
  /** @minLength 1 */
  callsign: string
  origin?: string | null
  destination?: string | null
  alternate?: string | null
  route?: string | null
  remarks?: string | null
  assignedSquawk?: string | null
  squawk?: string | null
  sid?: string | null
  /** @format int32 */
  clearedAltitude?: number | null
  /** @format int32 */
  finalAltitude?: number
  /** @format int32 */
  heading?: number | null
  aircraftCategory?: WeightCategory
  aircraftType?: string | null
  runway?: string | null
  capabilities?: string | null
  communicationType?: CommunicationType
  stand?: string | null
  tobt?: string | null
  /** @format int32 */
  height?: number
  /** @format double */
  latitude?: number
  /** @format double */
  longitude?: number
  tsat?: string | null
  /** @format int32 */
  sequence?: number | null
  cleared?: boolean
  controller?: string | null
  /** @minLength 1 */
  bay: string
}

export interface StripTransferRequestModel {
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  currentFrequency: string
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  toFrequency: string
  /** @minLength 1 */
  position: string
}

export interface ValidationProblemDetails {
  type?: string | null
  title?: string | null
  /** @format int32 */
  status?: number | null
  detail?: string | null
  instance?: string | null
  errors?: Record<string, string[]>
  [key: string]: any
}

export enum WeightCategory {
  Unknown = 'Unknown',
  Light = 'Light',
  Medium = 'Medium',
  Heavy = 'Heavy',
  SuperHeavy = 'SuperHeavy',
}

export type QueryParamsType = Record<string | number, any>
export type ResponseFormat = keyof Omit<Body, 'body' | 'bodyUsed'>

export interface FullRequestParams extends Omit<RequestInit, 'body'> {
  /** set parameter to `true` for call `securityWorker` for this request */
  secure?: boolean
  /** request path */
  path: string
  /** content type of request body */
  type?: ContentType
  /** query params */
  query?: QueryParamsType
  /** format of response (i.e. response.json() -> format: "json") */
  format?: ResponseFormat
  /** request body */
  body?: unknown
  /** base url */
  baseUrl?: string
  /** request cancellation token */
  cancelToken?: CancelToken
}

export type RequestParams = Omit<
  FullRequestParams,
  'body' | 'method' | 'query' | 'path'
>

export interface ApiConfig<SecurityDataType = unknown> {
  baseUrl?: string
  baseApiParams?: Omit<RequestParams, 'baseUrl' | 'cancelToken' | 'signal'>
  securityWorker?: (
    securityData: SecurityDataType | null,
  ) => Promise<RequestParams | void> | RequestParams | void
  customFetch?: typeof fetch
}

export interface HttpResponse<D extends unknown, E extends unknown = unknown>
  extends Response {
  data: D
  error: E
}

type CancelToken = Symbol | string | number

export enum ContentType {
  Json = 'application/json',
  FormData = 'multipart/form-data',
  UrlEncoded = 'application/x-www-form-urlencoded',
  Text = 'text/plain',
}

export class HttpClient<SecurityDataType = unknown> {
  public baseUrl: string = ''
  private securityData: SecurityDataType | null = null
  private securityWorker?: ApiConfig<SecurityDataType>['securityWorker']
  private abortControllers = new Map<CancelToken, AbortController>()
  private customFetch = (...fetchParams: Parameters<typeof fetch>) =>
    fetch(...fetchParams)

  private baseApiParams: RequestParams = {
    credentials: 'same-origin',
    headers: {},
    redirect: 'follow',
    referrerPolicy: 'no-referrer',
  }

  constructor(apiConfig: ApiConfig<SecurityDataType> = {}) {
    Object.assign(this, apiConfig)
  }

  public setSecurityData = (data: SecurityDataType | null) => {
    this.securityData = data
  }

  protected encodeQueryParam(key: string, value: any) {
    const encodedKey = encodeURIComponent(key)
    return `${encodedKey}=${encodeURIComponent(
      typeof value === 'number' ? value : `${value}`,
    )}`
  }

  protected addQueryParam(query: QueryParamsType, key: string) {
    return this.encodeQueryParam(key, query[key])
  }

  protected addArrayQueryParam(query: QueryParamsType, key: string) {
    const value = query[key]
    return value.map((v: any) => this.encodeQueryParam(key, v)).join('&')
  }

  protected toQueryString(rawQuery?: QueryParamsType): string {
    const query = rawQuery || {}
    const keys = Object.keys(query).filter(
      (key) => 'undefined' !== typeof query[key],
    )
    return keys
      .map((key) =>
        Array.isArray(query[key])
          ? this.addArrayQueryParam(query, key)
          : this.addQueryParam(query, key),
      )
      .join('&')
  }

  protected addQueryParams(rawQuery?: QueryParamsType): string {
    const queryString = this.toQueryString(rawQuery)
    return queryString ? `?${queryString}` : ''
  }

  private contentFormatters: Record<ContentType, (input: any) => any> = {
    [ContentType.Json]: (input: any) =>
      input !== null && (typeof input === 'object' || typeof input === 'string')
        ? JSON.stringify(input)
        : input,
    [ContentType.Text]: (input: any) =>
      input !== null && typeof input !== 'string'
        ? JSON.stringify(input)
        : input,
    [ContentType.FormData]: (input: any) =>
      Object.keys(input || {}).reduce((formData, key) => {
        const property = input[key]
        formData.append(
          key,
          property instanceof Blob
            ? property
            : typeof property === 'object' && property !== null
            ? JSON.stringify(property)
            : `${property}`,
        )
        return formData
      }, new FormData()),
    [ContentType.UrlEncoded]: (input: any) => this.toQueryString(input),
  }

  protected mergeRequestParams(
    params1: RequestParams,
    params2?: RequestParams,
  ): RequestParams {
    return {
      ...this.baseApiParams,
      ...params1,
      ...(params2 || {}),
      headers: {
        ...(this.baseApiParams.headers || {}),
        ...(params1.headers || {}),
        ...((params2 && params2.headers) || {}),
      },
    }
  }

  protected createAbortSignal = (
    cancelToken: CancelToken,
  ): AbortSignal | undefined => {
    if (this.abortControllers.has(cancelToken)) {
      const abortController = this.abortControllers.get(cancelToken)
      if (abortController) {
        return abortController.signal
      }
      return void 0
    }

    const abortController = new AbortController()
    this.abortControllers.set(cancelToken, abortController)
    return abortController.signal
  }

  public abortRequest = (cancelToken: CancelToken) => {
    const abortController = this.abortControllers.get(cancelToken)

    if (abortController) {
      abortController.abort()
      this.abortControllers.delete(cancelToken)
    }
  }

  public request = async <T = any, E = any>({
    body,
    secure,
    path,
    type,
    query,
    format,
    baseUrl,
    cancelToken,
    ...params
  }: FullRequestParams): Promise<HttpResponse<T, E>> => {
    const secureParams =
      ((typeof secure === 'boolean' ? secure : this.baseApiParams.secure) &&
        this.securityWorker &&
        (await this.securityWorker(this.securityData))) ||
      {}
    const requestParams = this.mergeRequestParams(params, secureParams)
    const queryString = query && this.toQueryString(query)
    const payloadFormatter = this.contentFormatters[type || ContentType.Json]
    const responseFormat = format || requestParams.format

    return this.customFetch(
      `${baseUrl || this.baseUrl || ''}${path}${
        queryString ? `?${queryString}` : ''
      }`,
      {
        ...requestParams,
        headers: {
          ...(requestParams.headers || {}),
          ...(type && type !== ContentType.FormData
            ? { 'Content-Type': type }
            : {}),
        },
        signal:
          (cancelToken
            ? this.createAbortSignal(cancelToken)
            : requestParams.signal) || null,
        body:
          typeof body === 'undefined' || body === null
            ? null
            : payloadFormatter(body),
      },
    ).then(async (response) => {
      const r = response as HttpResponse<T, E>
      r.data = null as unknown as T
      r.error = null as unknown as E

      const data = !responseFormat
        ? r
        : await response[responseFormat]()
            .then((data) => {
              if (r.ok) {
                r.data = data
              } else {
                r.error = data
              }
              return r
            })
            .catch((e) => {
              r.error = e
              return r
            })

      if (cancelToken) {
        this.abortControllers.delete(cancelToken)
      }

      if (!response.ok) throw data
      return data
    })
  }
}

/**
 * @title Vatsim.Scandinavia.FlightStrips.Host
 * @version 1.0
 */
export class Api<
  SecurityDataType extends unknown,
> extends HttpClient<SecurityDataType> {
  api = {
    /**
     * No description
     *
     * @tags Bay
     * @name ListBays
     * @request GET:/api/{airport}/bays
     */
    listBays: (airport: string, params: RequestParams = {}) =>
      this.request<Bay[], any>({
        path: `/api/${airport}/bays`,
        method: 'GET',
        format: 'json',
        ...params,
      }),
  }
  airport = {
    /**
     * No description
     *
     * @tags Coordination
     * @name ListCoordinationsForFrequency
     * @request GET:/{airport}/{session}/coordination/{frequency}
     */
    listCoordinationsForFrequency: (
      airport: string,
      session: string,
      frequency: string,
      params: RequestParams = {},
    ) =>
      this.request<CoordinationResponseModel[], ValidationProblemDetails>({
        path: `/${airport}/${session}/coordination/${frequency}`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Coordination
     * @name GetCoordination
     * @request GET:/{airport}/{session}/coordination/{id}
     */
    getCoordination: (
      airport: string,
      session: string,
      id: number,
      params: RequestParams = {},
    ) =>
      this.request<CoordinationResponseModel, ProblemDetails>({
        path: `/${airport}/${session}/coordination/${id}`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Coordination
     * @name AcceptCoordination
     * @request POST:/{airport}/{session}/coordination/{id}/accept
     */
    acceptCoordination: (
      airport: string,
      session: string,
      id: number,
      data: AcceptCoordinationRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, ValidationProblemDetails | ProblemDetails>({
        path: `/${airport}/${session}/coordination/${id}/accept`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Coordination
     * @name RejectCoordination
     * @request POST:/{airport}/{session}/coordination/{id}/reject
     */
    rejectCoordination: (
      airport: string,
      session: string,
      id: number,
      data: RejectCoordinationRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, ValidationProblemDetails | ProblemDetails>({
        path: `/${airport}/${session}/coordination/${id}/reject`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags OnlinePosition
     * @name ListOnlinePositions
     * @request GET:/{airport}/{session}/online-positions
     */
    listOnlinePositions: (
      airport: string,
      session: string,
      query?: {
        connected?: boolean
      },
      params: RequestParams = {},
    ) =>
      this.request<OnlinePositionResponseModel[], ValidationProblemDetails>({
        path: `/${airport}/${session}/online-positions`,
        method: 'GET',
        query: query,
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Runway
     * @name GetRunwayConfiguration
     * @request GET:/{airport}/{session}/runways
     */
    getRunwayConfiguration: (
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<RunwayConfigResponseModel, ProblemDetails>({
        path: `/${airport}/${session}/runways`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name ListStrips
     * @request GET:/{airport}/{session}/strips
     */
    listStrips: (
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<StripResponseModel[], any>({
        path: `/${airport}/${session}/strips`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name GetStrip
     * @request GET:/{airport}/{session}/strips/{callsign}
     */
    getStrip: (
      airport: string,
      session: string,
      callsign: string,
      params: RequestParams = {},
    ) =>
      this.request<StripResponseModel, ProblemDetails>({
        path: `/${airport}/${session}/strips/${callsign}`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name ClearStrip
     * @request POST:/{airport}/{session}/strips/{callsign}/clear
     */
    clearStrip: (
      airport: string,
      session: string,
      callsign: string,
      data: StripClearRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, ProblemDetails>({
        path: `/${airport}/${session}/strips/${callsign}/clear`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name MoveStrip
     * @request POST:/{airport}/{session}/strips/{callsign}/move
     */
    moveStrip: (
      airport: string,
      session: string,
      callsign: string,
      data: StripMoveRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, ProblemDetails>({
        path: `/${airport}/${session}/strips/${callsign}/move`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name AssumeStrip
     * @request POST:/{airport}/{session}/strips/{callsign}/assume
     */
    assumeStrip: (
      airport: string,
      session: string,
      callsign: string,
      data: StripAssumeRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, ProblemDetails>({
        path: `/${airport}/${session}/strips/${callsign}/assume`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strip
     * @name TransferStrip
     * @request POST:/{airport}/{session}/strips/{callsign}/transfer
     */
    transferStrip: (
      airport: string,
      session: string,
      callsign: string,
      data: StripTransferRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<CoordinationResponseModel, ProblemDetails>({
        path: `/${airport}/${session}/strips/${callsign}/transfer`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        format: 'json',
        ...params,
      }),
  }
  sessions = {
    /**
     * No description
     *
     * @tags Session
     * @name GetSessions
     * @request GET:/sessions
     */
    getSessions: (params: RequestParams = {}) =>
      this.request<SessionResponseModel, any>({
        path: `/sessions`,
        method: 'GET',
        format: 'json',
        ...params,
      }),
  }
}
