using Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs;

public interface IEventClient
{
    Task ReceiveControllerUpdate(ControllerUpdateModel controllerUpdate);

    Task ReceiveStripUpdate(StripUpdateModel stripUpdate);

    Task ReceiveAtisUpdate(AtisUpdateModel atisUpdate);

    Task ReceiveCoordinationUpdate(CoordinationUpdateModel coordinationUpdate);

    Task ReceiveControllerSectorsUpdate(SectorUpdateModel[] sectors);
}
