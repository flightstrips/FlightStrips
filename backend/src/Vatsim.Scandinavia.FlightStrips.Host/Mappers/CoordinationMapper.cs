using Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;
using Vatsim.Scandinavia.FlightStrips.Host.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Mappers;

public static class CoordinationMapper
{
    public static CoordinationResponseModel Map(Coordination coordination)
    {
        return new CoordinationResponseModel
        {
            Callsign = coordination.StripId.Callsign,
            FromFrequency = coordination.FromFrequency,
            ToFrequency = coordination.ToFrequency,
            State = coordination.State,
            Id = coordination.Id
        };
    }
}
