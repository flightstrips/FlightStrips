using System.Diagnostics.CodeAnalysis;
using Microsoft.Extensions.Logging;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Runways;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;

namespace Vatsim.Scandinavia.FlightStrips.Services;

public class OwnerService(ILogger<OwnerService> logger) : IOwnerService
{
    private static readonly Position[] _del = [Position.EKCH_DEL, Position.EKCH_D_GND, Position.EKCH_A_GND, Position.EKCH_C_TWR, Position.EKCH_D_TWR, Position.EKCH_A_TWR];

    private static readonly Position[] _gw = [Position.EKCH_C_TWR, Position.EKCH_D_TWR, Position.EKCH_A_TWR];

    private static readonly Position[] _aa = [Position.EKCH_A_GND, Position.EKCH_D_GND, Position.EKCH_C_TWR, Position.EKCH_A_TWR, Position.EKCH_D_TWR];
    private static readonly Position[] _ad = [Position.EKCH_D_GND, Position.EKCH_A_GND, Position.EKCH_C_TWR, Position.EKCH_D_TWR, Position.EKCH_A_TWR];

    private static readonly Dictionary<string, Dictionary<Sector, Position[]>> _owners = new()
    {
        {
            "22",
            new Dictionary<Sector, Position[]>
            {
                {Sector.DEL, _del},
                {Sector.AA, _aa},
                {Sector.AD, _ad},
                {Sector.GW, _gw},
                {Sector.GE, [Position.EKCH_D_TWR, Position.EKCH_A_TWR, Position.EKCH_C_TWR]},
                {Sector.TW, [Position.EKCH_D_TWR, Position.EKCH_A_TWR, Position.EKCH_C_TWR]},
                {Sector.TE, [Position.EKCH_A_TWR, Position.EKCH_D_TWR, Position.EKCH_C_TWR]}
            }
        },
        {
            "04",
            new Dictionary<Sector, Position[]>
            {
                {Sector.DEL, _del},
                {Sector.AA, _aa},
                {Sector.AD, _ad},
                {Sector.GW, _gw},
                {Sector.GE, [Position.EKCH_A_TWR, Position.EKCH_D_TWR, Position.EKCH_C_TWR]},
                {Sector.TW, [Position.EKCH_A_TWR, Position.EKCH_D_TWR, Position.EKCH_C_TWR]},
                {Sector.TE, [Position.EKCH_D_TWR, Position.EKCH_A_GND, Position.EKCH_C_TWR]}
            }
        },
        {
            "12",
            new Dictionary<Sector, Position[]>
            {
                {Sector.DEL, _del},
                {Sector.AA, _aa},
                {Sector.AD, _ad},
                {Sector.GW, _gw},
                {Sector.GE, [Position.EKCH_C_TWR, Position.EKCH_A_TWR, Position.EKCH_C_TWR]},
                {Sector.TW, [Position.EKCH_D_TWR, Position.EKCH_A_TWR, Position.EKCH_C_TWR]},
                {Sector.TE, [Position.EKCH_A_TWR, Position.EKCH_D_TWR, Position.EKCH_C_TWR]}
            }
        }
    };

    public OnlinePosition[] GetOwners(SessionId sessionId, RunwayConfig? runwayConfig, OnlinePosition[] onlinePositions)
    {
        if (sessionId.Airport != "EKCH") throw new InvalidOperationException("Airport not supported.");

        if (runwayConfig is null)
        {
            logger.RunwayConfigurationIsNull();
            return onlinePositions.Select(x =>
                new OnlinePosition {Id = x.Id, PrimaryFrequency = x.PrimaryFrequency, Sector = Sector.NONE}).ToArray();
        }

        var sectors = _owners[GetMainRunway(runwayConfig)];

        var positions = onlinePositions.Select(x => new Position(x.PrimaryFrequency)).Distinct().ToArray();

        var dict = positions.ToDictionary(x => x, _ => Sector.NONE);

        foreach (var sector in Enum.GetValues<Sector>())
        {
            if (sector is Sector.NONE) continue;
            var pos = sectors[sector].FirstOrDefault(x => positions.Contains(x));
            if (pos == default) continue;

            dict[pos] |= sector;
        }

        var result = new OnlinePosition[onlinePositions.Length];

        for (var i = 0; i < onlinePositions.Length; i++)
        {
            var pos = onlinePositions[i];
            result[i] = new OnlinePosition
            {
                Id = pos.Id,
                PrimaryFrequency = pos.PrimaryFrequency,
                Sector = dict[new Position(pos.PrimaryFrequency)]
            };
        }

        return result;
    }

    private static string PositionToString(OnlinePosition position)
    {
        return $"{position.PrimaryFrequency}: {position.Sector.ToString()}";
    }

    private static readonly string[] _runway12 = ["12", "30"];
    private static readonly string[] _runway22 = ["22L", "22R"];
    private static readonly string[] _runway04 = ["04L", "04R"];

    private static string GetMainRunway(RunwayConfig config)
    {
        if (_runway12.Contains(config.Departure, StringComparer.OrdinalIgnoreCase) ||
            _runway12.Contains(config.Arrival, StringComparer.OrdinalIgnoreCase))
        {
            return "12";
        }

        if (_runway22.Contains(config.Departure, StringComparer.OrdinalIgnoreCase) ||
            _runway22.Contains(config.Arrival, StringComparer.OrdinalIgnoreCase))
        {
            return "22";
        }


        if (_runway04.Contains(config.Departure, StringComparer.OrdinalIgnoreCase) ||
            _runway04.Contains(config.Arrival, StringComparer.OrdinalIgnoreCase))
        {
            return "04";
        }

        throw new InvalidOperationException("Unable to determine runway configuration");
    }
}
