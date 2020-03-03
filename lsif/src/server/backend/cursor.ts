import * as sqliteModels from '../../shared/models/sqlite'

/** Context describing the current request for paginated results. */
export interface ReferencePaginationContext {
    /** The maximum number of remote dumps to search. */
    limit: number

    /** Context describing the next page of results. */
    cursor?: ReferencePaginationCursor
}

/** Context describing the next page of results. */
export type ReferencePaginationCursor = SameDumpReferenceCursor | RemoteDumpReferenceCursor

/** The cursor phase is a tag that indicates the shape of the object. */
export type ReferencePaginationPhase = 'same-dump' | 'same-dump-monikers' | 'same-repo' | 'remote-repo'

/** Fields common to all reference pagination cursors. */
interface ReferencePaginationCursorCommon {
    /** The identifier of the dump that contains the target range. */
    dumpId: number

    /** The phase of the pagination. */
    phase: ReferencePaginationPhase
}

/** Bookkeeping data for the part of the reference result sets that deal with the initial dump. */
export interface SameDumpReferenceCursor extends ReferencePaginationCursorCommon {
    phase: 'same-dump' | 'same-dump-monikers'

    /** The (database-relative) document path containing the symbol ranges. */
    path: string

    /** A normalized list of monikers attached to the symbol ranges. */
    monikers: sqliteModels.MonikerData[]
}

/** Bookkeeping data for the part of the reference result sets that deal with additional dumps. */
export interface RemoteDumpReferenceCursor extends ReferencePaginationCursorCommon {
    phase: 'same-repo' | 'remote-repo'

    /** The identifier of the moniker that has remote results. */
    identifier: string

    /** The scheme of the moniker that has remote results. */
    scheme: string

    /** The name of the package that has remote results. */
    name: string

    /** The version of the package that has remote results. */
    version: string | null

    /** The number of dumps to skip. */
    offset: number
}

/** Create an initial pagination cursor. */
export function makeInitialSameDumpCursor(args: {
    dumpId: number
    path: string
    monikers: sqliteModels.MonikerData[]
}): ReferencePaginationCursor {
    return { phase: 'same-dump', ...args }
}

/** Create a pagination cursor at the beginning of the same dump monikers phase. */
export function makeInitialSameDumpMonikersCursor(previousCursor: SameDumpReferenceCursor): ReferencePaginationCursor {
    return { ...previousCursor, phase: 'same-dump-monikers' }
}

/** Create a pagination cursor at the beginning of the same repo phase. */
export function makeInitialSameRepoCursor(
    previousCursor: SameDumpReferenceCursor,
    { scheme, identifier }: sqliteModels.MonikerData,
    { name, version }: sqliteModels.PackageInformationData
): ReferencePaginationCursor {
    return {
        ...previousCursor,
        phase: 'same-repo',
        scheme,
        identifier,
        name,
        version,
        offset: 0,
    }
}

/** Create a pagination cursor at the beginning of the remote repo phase. */
export function makeInitialRemoteRepoCursor(previousCursor: RemoteDumpReferenceCursor): ReferencePaginationCursor {
    return { ...previousCursor, phase: 'remote-repo', offset: 0 }
}
