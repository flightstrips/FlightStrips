using StackExchange.Redis;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.Redis;

public class RedisStripRepository : IStripRepository
{
    private readonly IDatabase _database;

    public RedisStripRepository(IDatabase database)
    {
        _database = database;
    }

    private const string DestinationField = "destination";
    private const string OriginField = "origin";
    private const string ClearedField = "cleared";
    private const string ControllerField = "controller";
    private const string NextControllerField = "nextController";
    private const string StateField = "state";

    public async Task CreateAsync(StripCreateRequest createRequest)
    {
        var key = new RedisKey(createRequest.Id.ToString());

        if (await _database.KeyExistsAsync(createRequest.Id.ToString()))
        {
            throw new InvalidOperationException();
        }

        var entries = new HashEntry[]
        {
            new (DestinationField, createRequest.Destination),
            new (OriginField, createRequest.Origin),
            new (ClearedField, createRequest.Cleared),
            new (ControllerField, createRequest.Controller),
            new (NextControllerField, createRequest.NextController),
            new (StateField, (int)createRequest.State)
        };

        await _database.HashSetAsync(key, entries);
    }

    public Task DeleteAsync(StripId stripId)
    {
        return _database.KeyDeleteAsync(stripId.ToString());
    }

    public async Task<Strip?> GetAsync(StripId stripId)
    {
        if (!await _database.KeyExistsAsync(stripId.ToString()))
        {
            return null;
        }

        var values = await _database.HashGetAsync(stripId.ToString(),
            new RedisValue[] { DestinationField, OriginField, ClearedField, ControllerField, NextControllerField, StateField });

        return new Strip
        {
            Callsign = stripId.Callsign,
            Destination = values[0],
            Origin = values[1],
            Cleared = (bool)values[2],
            Controller = values[3],
            NextController = values[4],
            State = (StripState)(int)values[5]
        };
    }

    public async Task SetSequenceAsync(StripId stripId, int? sequence)
    {
        var sortedSetKey = GetSortedSetKey(stripId);
        var current = await _database.SortedSetScoreAsync(sortedSetKey, stripId.Callsign);

        if (current.HasValue)
        {
            await _database.SortedSetRemoveAsync(sortedSetKey, stripId.Callsign);
        }

        if (current.HasValue && sequence.HasValue && sequence.Value > current.Value)
        {
            // Shift up elements between the old and the new score (both inclusive).
            foreach (var entry in await _database.SortedSetRangeByScoreWithScoresAsync(sortedSetKey, current.Value + 1, sequence.Value))
            {
                await _database.SortedSetAddAsync(sortedSetKey, entry.Element, entry.Score - 1);
            }
        }
        else if (current.HasValue && sequence.HasValue && sequence.Value < current.Value)
        {
            // Shift down elements between the new and the old score (both inclusive).
            foreach (var entry in await _database.SortedSetRangeByScoreWithScoresAsync(sortedSetKey, sequence.Value, current.Value - 1))
            {
                await _database.SortedSetAddAsync(sortedSetKey, entry.Element, entry.Score + 1);
            }
        }
        else if (current.HasValue && !sequence.HasValue)
        {
            // If we are removing an item, shift down elements above the old score.
            foreach (var entry in await _database.SortedSetRangeByScoreWithScoresAsync(sortedSetKey, current.Value + 1, double.PositiveInfinity))
            {
                await _database.SortedSetAddAsync(sortedSetKey, entry.Element, entry.Score - 1);
            }
        }
        else if (!current.HasValue && sequence.HasValue)
        {
            // If we are adding a new item, shift up elements above (or equal to) the new score.
            foreach (var entry in await _database.SortedSetRangeByScoreWithScoresAsync(sortedSetKey, sequence.Value, double.PositiveInfinity))
            {
                await _database.SortedSetAddAsync(sortedSetKey, entry.Element, entry.Score + 1);
            }
        }

        if (sequence.HasValue)
        {
            await _database.SortedSetAddAsync(sortedSetKey, stripId.Callsign, sequence.Value);
        }
    }

    private static string GetSortedSetKey(StripId id)
    {
        return $"DSQ:{id.Session}:{id.Airport}";
    }
}