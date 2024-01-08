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
  default: boolean
  callsignFilter: string[]
}

export interface Coordination {
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

export interface HttpValidationProblemDetails {
  type?: string | null
  title?: string | null
  /** @format int32 */
  status?: number | null
  detail?: string | null
  instance?: string | null
  errors?: Record<string, string[]>
  [key: string]: any
}

export interface OnlinePosition {
  /** @minLength 1 */
  positionId: string
  /** @minLength 1 */
  primaryFrequency: string
}

export interface OnlinePositionCreateRequestModel {
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  frequency: string
}

export interface Position {
  name?: string | null
  frequency?: string | null
}

export interface RejectCoordinationRequestModel {
  /**
   * @minLength 1
   * @pattern ^\d{3}\.\d{3}$
   */
  frequency: string
}

export interface Strip {
  /** @minLength 1 */
  callsign: string
  origin?: string | null
  destination?: string | null
  /** @format int32 */
  sequence?: number | null
  state?: StripState
  cleared?: boolean
  positionFrequency?: string | null
  /** @minLength 1 */
  bay: string
  /** @format date-time */
  lastUpdated: string
}

export interface StripAssumeRequestModel {
  /** @minLength 1 */
  frequency: string
  force: boolean
}

export interface StripMoveRequestModel {
  /** @minLength 1 */
  bay: string
  /** @format int32 */
  sequence?: number | null
}

export enum StripState {
  None = 'None',
  Startup = 'Startup',
  Push = 'Push',
  Taxi = 'Taxi',
  Deice = 'Deice',
  Lineup = 'Lineup',
  Depart = 'Depart',
  Arrival = 'Arrival',
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
}

export interface UpsertBayRequestModel {
  default: boolean
  callsignFilter?: string[] | null
}

export interface UpsertPositionRequestModel {
  /**
   * @minLength 1
   * @maxLength 50
   */
  name: string
}

export interface UpsertStripRequestModel {
  origin?: string | null
  destination?: string | null
  state?: StripState
  cleared?: boolean
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
     * @tags Bays
     * @name UpsertBay
     * @summary Create or update bay
     * @request PUT:/api/{airport}/bays/{name}
     */
    upsertBay: (
      name: string,
      airport: string,
      data: UpsertBayRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, any>({
        path: `/api/${airport}/bays/${name}`,
        method: 'PUT',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Bays
     * @name DeleteBay
     * @summary Delete bay.
     * @request DELETE:/api/{airport}/bays/{name}
     */
    deleteBay: (name: string, airport: string, params: RequestParams = {}) =>
      this.request<void, any>({
        path: `/api/${airport}/bays/${name}`,
        method: 'DELETE',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Bays
     * @name ListBays
     * @summary Retrieve bays
     * @request GET:/api/{airport}/bays
     */
    listBays: (airport: string, params: RequestParams = {}) =>
      this.request<Bay[], any>({
        path: `/api/${airport}/bays`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * @description List coordination ongoing for frequency
     *
     * @tags Coordination
     * @name ListCoordination
     * @request GET:/api/{airport}/{session}/coordination/{frequency}
     */
    listCoordination: (
      frequency: string,
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<Coordination[], HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/coordination/${frequency}`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * @description Accept coordination
     *
     * @tags Coordination
     * @name AcceptCoordination
     * @request POST:/api/{airport}/{session}/coordination/{id}/accept
     */
    acceptCoordination: (
      id: number,
      airport: string,
      session: string,
      data: AcceptCoordinationRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails | void>({
        path: `/api/${airport}/${session}/coordination/${id}/accept`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Reject coordination
     *
     * @tags Coordination
     * @name RejectCoordination
     * @request POST:/api/{airport}/{session}/coordination/{id}/reject
     */
    rejectCoordination: (
      id: number,
      airport: string,
      session: string,
      data: RejectCoordinationRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails | void>({
        path: `/api/${airport}/${session}/coordination/${id}/reject`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Online Positions
     * @name CreateOnlinePosition
     * @request POST:/api/{airport}/{session}/online-positions/{id}
     */
    createOnlinePosition: (
      id: string,
      airport: string,
      session: string,
      data: OnlinePositionCreateRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/online-positions/${id}`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Online Positions
     * @name DeleteOnlinePosition
     * @request DELETE:/api/{airport}/{session}/online-positions/{id}
     */
    deleteOnlinePosition: (
      id: string,
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/online-positions/${id}`,
        method: 'DELETE',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Online Positions
     * @name ListOnlinePositions
     * @request GET:/api/{airport}/{session}/online-positions
     */
    listOnlinePositions: (
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<OnlinePosition[], HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/online-positions`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Positions
     * @name UpsertPosition
     * @summary Create or update position
     * @request PUT:/api/{airport}/positions/{frequency}
     */
    upsertPosition: (
      frequency: string,
      airport: string,
      data: UpsertPositionRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/positions/${frequency}`,
        method: 'PUT',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Positions
     * @name DeletePosition
     * @summary Delete position
     * @request DELETE:/api/{airport}/positions/{frequency}
     */
    deletePosition: (
      frequency: string,
      airport: string,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/positions/${frequency}`,
        method: 'DELETE',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Positions
     * @name ListPositions
     * @summary List positions
     * @request GET:/api/{airport}/positions
     */
    listPositions: (airport: string, params: RequestParams = {}) =>
      this.request<Position[], any>({
        path: `/api/${airport}/positions`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strips
     * @name GetStrip
     * @summary Gets a strip from identifier.
     * @request GET:/api/{airport}/{session}/strips/{callsign}
     */
    getStrip: (
      callsign: string,
      airport: string,
      session: string,
      params: RequestParams = {},
    ) =>
      this.request<Strip, HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/strips/${callsign}`,
        method: 'GET',
        format: 'json',
        ...params,
      }),

    /**
     * @description Create strip if it does not exist, otherwise update
     *
     * @tags Strips
     * @name UpsertStrip
     * @summary Upsert strip
     * @request POST:/api/{airport}/{session}/strips/{callsign}
     */
    upsertStrip: (
      callsign: string,
      airport: string,
      session: string,
      data: UpsertStripRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/strips/${callsign}`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * No description
     *
     * @tags Strips
     * @name MoveStrip
     * @summary Move strip to bay and set sequence
     * @request POST:/api/{airport}/{session}/strips/{callsign}/move
     */
    moveStrip: (
      callsign: string,
      airport: string,
      session: string,
      data: StripMoveRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails>({
        path: `/api/${airport}/${session}/strips/${callsign}/move`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Assume a strip.
     *
     * @tags Strips
     * @name AssumeStrip
     * @request POST:/api/{airport}/{session}/strips/{callsign}/assume
     */
    assumeStrip: (
      callsign: string,
      airport: string,
      session: string,
      data: StripAssumeRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<void, HttpValidationProblemDetails | void>({
        path: `/api/${airport}/${session}/strips/${callsign}/assume`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        ...params,
      }),

    /**
     * @description Transfer a strip
     *
     * @tags Strips
     * @name TransferStrip
     * @request POST:/api/{airport}/{session}/strips/{callsign}/transfer
     */
    transferStrip: (
      callsign: string,
      airport: string,
      session: string,
      data: StripTransferRequestModel,
      params: RequestParams = {},
    ) =>
      this.request<Coordination, HttpValidationProblemDetails | void>({
        path: `/api/${airport}/${session}/strips/${callsign}/transfer`,
        method: 'POST',
        body: data,
        type: ContentType.Json,
        format: 'json',
        ...params,
      }),
  }
}
