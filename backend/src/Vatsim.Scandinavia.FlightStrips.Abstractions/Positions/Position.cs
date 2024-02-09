using System.Diagnostics.CodeAnalysis;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;

[SuppressMessage("Naming", "CA1707:Identifiers should not contain underscores")]
public record struct Position(string Frequency)
{
    public static readonly Position EKCH_S_GND = new ("");
    public static readonly Position EKCH_A_GND = new ("121.630");
    public static readonly Position EKCH_D_GND = new ("121.730");
    public static readonly Position EKCH_A_TWR = new ("118.105");
    public static readonly Position EKCH_D_TWR = new ("119.335");
    public static readonly Position EKCH_C_TWR = new ("118.580");
    public static readonly Position EKCH_DEL = new ("119.905");
};
