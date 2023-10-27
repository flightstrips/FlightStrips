using System.ComponentModel.DataAnnotations;
using System.Diagnostics.CodeAnalysis;

namespace Vatsim.Scandinavia.FlightStrips.Host.Attributes;

[AttributeUsage(AttributeTargets.Property | AttributeTargets.Field | AttributeTargets.Parameter)]
public class CallsignAttribute : RegularExpressionAttribute
{
    [StringSyntax(StringSyntaxAttribute.Regex)]
    private const string Regex = "^[A-Za-z0-9]*$";

    public CallsignAttribute() : base(Regex)
    {
    }
}
