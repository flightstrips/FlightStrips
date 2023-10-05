using Vatsim.Scandinavia.FlightStrips.Abstractions.Entities;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Interfaces;

public interface IBayService
{
    Bay CreateBay(string name);
    Bay UpdateBay(Bay updatedBay);
    void DeleteBay(Guid id); 
}