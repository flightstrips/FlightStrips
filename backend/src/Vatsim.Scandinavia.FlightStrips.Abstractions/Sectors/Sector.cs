// ReSharper disable InconsistentNaming
namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Sectors;

[Flags]
public enum Sector
{
    NONE = 0,
    DEL =  1 << 0,
    AA =   1 << 1,
    AD =   1 << 2,
    GW =   1 << 3,
    GE =   1 << 4,
    TW =   1 << 5,
    TE =   1 << 6

}

public static class Sectors
{
    public const Sector All = Sector.DEL | Sector.AA | Sector.AD | Sector.GW | Sector.GE | Sector.TW | Sector.TE;
}
