using Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Masters;

public interface IMasterService
{
    bool IsMaster(OnlinePositionId id);
    bool SetMaster(OnlinePositionId id);
}
